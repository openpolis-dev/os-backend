package rewards

import (
	"bytes"
	gob "encoding/gob"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/service"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
)

const MintRewardDetailTemplate = "SeeDAO %s 治理挖矿收益"

func ApproveMintReward(ctx *gin.Context) {
	// Get current reward records
	//user, enforcer, db, _ := api.ForContext(ctx)
	user, _, db, _ := api.ForContext(ctx)

	// TODO: Enforcer check permission

	currentSeason, err := service.GetCurrentSeason(db)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	metaforoRewardsBytes, err := storage.GetCachedData(storage.MetaforoRewardCacheKey(currentSeason.Idx))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	buf := bytes.NewBuffer(metaforoRewardsBytes)
	bufDecoder := gob.NewDecoder(buf)
	log.Error().Msgf("TTT: Read buf size: %d", buf.Len())
	var metaforoRewards map[string]string
	err = bufDecoder.Decode(&metaforoRewards)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	cityHallProject, err := model.GetCityHallProject(db)

	// Create app bundle and applications for each records
	err = db.Transaction(func(tx *gorm.DB) error {
		appBundle := model.AppBundle{
			AppRecords:   nil,
			Comment:      fmt.Sprintf(MintRewardDetailTemplate, currentSeason.Name),
			Submitter:    model.FormatUserWallet(user.Wallet),
			EntityType:   "project",
			EntityId:     cityHallProject.ID,
			SeasonId:     currentSeason.ID,
			Season:       *currentSeason,
			State:        model.ApplicationStateOpen,
			Type:         model.ApplicationNewReward,
			ShadowRecord: false,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		log.Error().Msgf("TTT: app bundle: %+v", appBundle)
		err = tx.Save(&appBundle).Error
		if err != nil {
			log.Error().Msgf("create app bundle error: %+v", err)
			return err
		}

		var appRcds []*model.Application
		for wallet, rewardAmountStr := range metaforoRewards {
			rewardAmount, err := decimal.NewFromString(rewardAmountStr)
			if err != nil {
				log.Error().Msgf("parse reward amount error, amount str: %s, err: %+v", rewardAmountStr, err)
				return err
			}

			appRcds = append(appRcds, &model.Application{
				Type:             model.ApplicationNewReward,
				Applicant:        "",
				State:            model.ApplicationStateOpen,
				CreatedAt:        time.Now(),
				UpdatedAt:        time.Now(),
				DetailedType:     fmt.Sprintf(MintRewardDetailTemplate, currentSeason.Name),
				Comment:          "",
				AssetName:        "SCR",
				AssetAmount:      rewardAmount,
				TargetUserWallet: model.FormatUserWallet(wallet),
				EntityType:       "project",
				EntityId:         cityHallProject.ID,
				SeasonId:         currentSeason.ID,
				BundleId:         appBundle.ID,
			})
		}

		err = tx.Save(&appRcds).Error
		if err != nil {
			return err
		}

		// Mark season metaforo credit confirmed
		currentSeason.MintRewardConfirmed = true
		currentSeason.MintRewardConfirmedAt = time.Now().UnixMilli()
		currentSeason.MintRewardAppBundleId = appBundle.ID
		return tx.Save(currentSeason).Error
	})

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func SnapshotSeed(ctx *gin.Context) {
	// Get current reward records
	//user, enforcer, db, _ := api.ForContext(ctx)
	user, _, db, _ := api.ForContext(ctx)

	// TODO: check permission of user

	currentSeason, err := service.GetCurrentSeason(db)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	currentSeason.SeedSnapshotSaved = true
	currentSeason.SeedSnapshotAt = time.Now().UnixMilli()
	currentSeason.SeedSnapshotSubmitter = model.FormatUserWallet(user.Wallet)
	err = db.Save(currentSeason).Error

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
	} else {
		ctx.JSON(http.StatusOK, api.Success(fmt.Sprintf("SEED snapshoted at %d", currentSeason.SeedSnapshotAt)))
	}
}
