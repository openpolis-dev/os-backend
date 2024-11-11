package rewards

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/api/data_srv"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
)

const MintRewardDetailTemplate = "SeeDAO %s 治理挖矿收益"

func ApproveMintReward(ctx *gin.Context) {
	err := doApproveMintReward(ctx)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("approve mint reward error: %+v", err)))
		return
	}
	ctx.JSON(http.StatusOK, api.Success(nil))
}

func doApproveMintReward(ctx *gin.Context) error {
	// Get current reward records
	//user, enforcer, db, _ := api.ForContext(ctx)
	user, _, db, _ := api.ForContext(ctx)

	// TODO: Enforcer check permission

	currentSeason, err := model.GetCurrentSeason(db)
	if err != nil {
		log.Error().Msgf("fetch current season error: %v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		return err
	}

	if currentSeason.MintRewardConfirmed {
		errMsg := fmt.Errorf("mint reward for season %d has already confirmed at %d", currentSeason.Idx, currentSeason.MintRewardConfirmedAt)
		log.Error().Msgf(errMsg.Error())
		return errMsg
	}

	var metaforoRewards map[string]string
	metaforoRewardsBytes, err := storage.GetCachedData(storage.MetaforoRewardCacheKey(currentSeason.Idx))
	if err != nil {
		if errors.Is(err, bigcache.ErrEntryNotFound) {
			// The error is cache not found, calculate the reward directly
			log.Debug().Msgf("mint reward cache not found, calculate it for season: %d", currentSeason.Idx)
			mintResult, err := data_srv.CalcMintRewards(ctx, db, currentSeason)
			if err != nil {
				log.Error().Msgf("calc mint rewards error: %+v", err)
				sdk.LogServerErrorToSentry(ctx, err)
				return err
			}
			metaforoRewards = mintResult.MintRewardData
		} else {
			log.Error().Msgf("fetch metaforo rewards error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			return err
		}
	} else {
		buf := bytes.NewBuffer(metaforoRewardsBytes)
		bufDecoder := gob.NewDecoder(buf)
		err = bufDecoder.Decode(&metaforoRewards)
		if err != nil {
			log.Error().Msgf("decode metaforo rewards error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			return err
		}
	}

	cityHallProject, err := model.GetCityHallProject(db)

	if len(metaforoRewards) > 0 {
		// Create app bundle and applications for each records if there are rewards
		err = db.Transaction(func(tx *gorm.DB) error {
			appBundle := model.AppBundle{
				AppRecords:   nil,
				Comment:      fmt.Sprintf(MintRewardDetailTemplate, currentSeason.Name),
				Applicant:    common.FormatUserWallet(user.Wallet),
				EntityType:   "project",
				EntityId:     cityHallProject.ID,
				SeasonId:     currentSeason.ID,
				Season:       *currentSeason,
				State:        model.ApplicationStateOpen,
				Type:         model.ApplicationNewReward,
				ShadowRecord: false,
				CreatedAt:    time.Now().In(internal.ProjectTimezone),
				UpdatedAt:    time.Now().In(internal.ProjectTimezone),
				CreateTs:     model.GetCurrentUtcEpochSecond(),
				UpdateTs:     model.GetCurrentUtcEpochSecond(),
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
					SubType:          "MintRewards",
					Applicant:        common.FormatUserWallet(user.Wallet),
					State:            model.ApplicationStateOpen,
					CreatedAt:        time.Now(),
					UpdatedAt:        time.Now(),
					CreateTs:         model.GetCurrentUtcEpochSecond(),
					UpdateTs:         model.GetCurrentUtcEpochSecond(),
					DetailedType:     fmt.Sprintf(MintRewardDetailTemplate, currentSeason.Name),
					Comment:          "",
					AssetName:        "SCR",
					AssetAmount:      rewardAmount,
					TargetUserWallet: common.FormatUserWallet(wallet),
					EntityType:       "project",
					EntityId:         cityHallProject.ID,
					SeasonId:         currentSeason.ID,
					BundleId:         appBundle.ID,
				})
			}

			if len(appRcds) > 0 {
				err = tx.Save(&appRcds).Error
				if err != nil {
					log.Error().Msgf("create application error: %+v", err)
					return err
				}
			}

			// Mark season metaforo credit confirmed
			currentSeason.MintRewardConfirmed = true
			currentSeason.MintRewardConfirmedAt = time.Now().UnixMilli()
			currentSeason.MintRewardAppBundleId = appBundle.ID
			return tx.Save(currentSeason).Error
		})

		if err != nil {
			log.Error().Msgf("approve mint reward error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			return err
		}
	} else {
		log.Warn().Msgf("no metaforo rewards found for season %d, no application and bundle will be created", currentSeason.Idx)
	}

	return nil
}

func SnapshotSeed(ctx *gin.Context) {
	seedSnapshotAt, err := doSnapshotSeed(ctx)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("snapshot seed error: %+v", err)))
		return
	}
	ctx.JSON(http.StatusOK, api.Success(fmt.Sprintf("SEED snapshoted at %d", seedSnapshotAt)))
}

func doSnapshotSeed(ctx *gin.Context) (int64, error) {
	// Get current reward records
	//user, enforcer, db, _ := api.ForContext(ctx)
	user, _, db, _ := api.ForContext(ctx)

	currentSeason, err := model.GetCurrentSeason(db)
	if err != nil {
		log.Error().Msgf("fetch current season error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		return 0, err
	}

	if currentSeason.SeedSnapshotSaved {
		errMsg := fmt.Errorf("seed snapshot for season %d has already saved at %d", currentSeason.Idx, currentSeason.SeedSnapshotAt)
		log.Error().Msgf(errMsg.Error())
		return 0, errMsg
	}

	// TODO: check permission of user

	currentSeason.SeedSnapshotSaved = true
	currentSeason.SeedSnapshotAt = time.Now().Unix()
	currentSeason.SeedSnapshotSubmitter = common.FormatUserWallet(user.Wallet)
	err = db.Save(currentSeason).Error

	if err != nil {
		log.Error().Msgf("save current season error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		return 0, err
	}
	return currentSeason.SeedSnapshotAt, nil
}

func ApproveMintAndSnapshotSeed(ctx *gin.Context) {
	err := doApproveMintReward(ctx)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("approve mint and snapshot seed error: %+v", err)))
		return
	}
	seedSnapshotAt, err := doSnapshotSeed(ctx)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("approve mint and snapshot seed error: %+v", err)))
		return
	}
	ctx.JSON(http.StatusOK, api.Success(fmt.Sprintf("SEED snapshoted at %d", seedSnapshotAt)))
}
