package app_bundle

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/service"
	"gorm.io/gorm"
)

type AppBundleResponseRecord struct {
	SeasonName string               `json:"season_name"`
	Records    []*model.Application `json:"records"`
	Entity     struct {
		Id   uint   `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"entity"`
	Submitter  string                 `json:"submitter"`
	SubmitDate time.Time              `json:"submit_date"`
	Reviewer   string                 `json:"reviewer"`
	Comment    string                 `json:"comment"`
	State      model.ApplicationState `json:"state"`
	Assets     []struct {
		Name   string `json:"name"`
		Amount string `json:"amount"`
	} `json:"assets"`
}

func BuildResponseFromDatabaseSearchResult() {

}

// ListAppBundle returns application bundles with passed in query types
// TODO: Update query params
//
//	@Summary		List all application bundles match the query params
//	@Router			/app_bundles [get]
//	@Tags			app_bundle
//	@Param			status		query		string	false	"status of application bundle"	Enum(open approved rejected processing completed)
//	@Param			page		query		string	false	"which page"
//	@Param			size		query		string	false	"size of each page"
//	@Param			sort_field	query		string	false	"sort by which field"
//	@Param			sort_order	query		string	false	"order of sort"	Enum(asc desc)
//
//	@Success		200			{object}	AppBundleResponseRecord
func ListAppBundle(ctx *gin.Context) {
	var err error
	db := api.ForContextOnlyDB(ctx)

	queryParams := model.ListAppBundleQueryParams{}

	appBundleRecords, total, err := model.QueryAppBundleRecords(db, &queryParams)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("query result error: %+v", err),
		})
		return
	}

	respRcds := lo.Map(appBundleRecords, func(jointAppBundleEntityRcd model.JointAppBundleEntityRslt, index int) AppBundleResponseRecord {
		assetSummary := make(map[string]decimal.Decimal)

		var appRcds []*model.Application
		err = db.Model(&model.Application{}).
			Where("bundle_id = ?", jointAppBundleEntityRcd.AppBundle.ID).
			Where("state = ?", model.ApplicationStateOpen).
			Find(&appRcds).
			Error
		if err != nil {
			log.Error().Msgf("query application error: %+v", err)
			return AppBundleResponseRecord{}
		}

		// Summarize the assets in app bundle
		for _, appRcd := range appRcds {
			detailedData := model.NewRewardApplicationDetailedData{}
			err := json.Unmarshal(appRcd.DetailedData, &detailedData)
			if err != nil {
				log.Error().Msgf("parse application detailed data error: %+v, detailed data: %q", err, appRcd.DetailedData)
				return AppBundleResponseRecord{}
			}

			for assetName, assetRcd := range detailedData.Assets {
				model.SetDefaultMapValue(assetSummary, assetName, decimal.Zero)
				assetSummary[assetName] = assetSummary[assetName].Add(assetRcd.Amount)
			}
		}

		return AppBundleResponseRecord{
			SeasonName: jointAppBundleEntityRcd.AppBundle.Season.Name,
			Records:    jointAppBundleEntityRcd.AppBundle.AppRecords,
			Entity: struct {
				Id   uint   `json:"id"`
				Name string `json:"name"`
				Type string `json:"type"`
			}{
				Id:   jointAppBundleEntityRcd.AppBundle.EntityId,
				Name: jointAppBundleEntityRcd.EntityName,
				Type: jointAppBundleEntityRcd.AppBundle.EntityType,
			},
			Submitter:  jointAppBundleEntityRcd.AppBundle.Submitter,
			SubmitDate: jointAppBundleEntityRcd.AppBundle.CreatedAt,
			Comment:    jointAppBundleEntityRcd.AppBundle.Comment,
			State:      jointAppBundleEntityRcd.AppBundle.State,
			Assets: lo.MapToSlice(assetSummary, func(assetName string, amount decimal.Decimal) struct {
				Name   string `json:"name"`
				Amount string `json:"amount"`
			} {
				return struct {
					Name   string `json:"name"`
					Amount string `json:"amount"`
				}{
					Name:   assetName,
					Amount: amount.String(),
				}
			}),
		}
	})

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  queryParams.Page,
		Size:  queryParams.Size,
		Total: total,
		Rows:  respRcds,
	}))
}

// CreateAppBundle returns application bundles with passed in query types
//
//	@Summary		List all application bundles match the query params
//	@Router			/app_bundles [post]
//	@Tags			app_bundle
//	@Param			request body model.NewAppBundleRequest true "New application bundle request"
//
//	@Success		201			{string}	AppBundleResponseRecord
func CreateAppBundle(ctx *gin.Context) {
	var newAppBundleReq model.NewAppBundleRequest
	if err := ctx.BindJSON(&newAppBundleReq); err != nil {
		if err != nil {
			ctx.JSON(http.StatusBadRequest, api.Reply{
				Code: -1,
				Msg:  fmt.Sprintf("passed in data error: %+v", err),
			})
		}
		return
	}

	user, enforcer, db, _ := api.ForContext(ctx)

	// Check permission, using ActCreateApplication for now, can be changed to new permission if required
	// TODO: Merge to separated functions
	obj := lo.
		If(newAppBundleReq.Entity == "project", fmt.Sprintf("%s%d", api.ObjProjPrefix, newAppBundleReq.EntityId)).
		ElseIf(newAppBundleReq.Entity == "guild", fmt.Sprintf("%s%d", api.ObjGuildPrefix, newAppBundleReq.EntityId)).
		Else("")
	ok, err := enforcer.Enforce(user.Wallet, obj, api.ActCreateApplication)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	// TODO: Need confirm about season number for application bundles
	seasonRecord, err := service.GetCurrentSeason(db)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		appBundle := model.AppBundle{
			Comment:    newAppBundleReq.Comment,
			Submitter:  user.Wallet,
			EntityType: newAppBundleReq.Entity,
			EntityId:   newAppBundleReq.EntityId,
			SeasonId:   seasonRecord.ID,
			Season:     *seasonRecord,
			State:      model.ApplicationStateOpen,
			CreatedAt:  time.Now().In(internal.ProjectTimezone),
			UpdatedAt:  time.Now().In(internal.ProjectTimezone),
		}

		appBundle.AppRecords = lo.Map(newAppBundleReq.Records, func(appRcd *model.NewApplicationRequest, index int) *model.Application {
			rewardDetailedData := model.NewRewardApplicationDetailedData{
				TargetUserWallet: appRcd.TargetUserWallet,
				Assets: map[string]model.NewRewardAssetRecord{
					appRcd.AssetName: {
						AssetName: appRcd.AssetName,
						Amount:    appRcd.Amount,
					},
				},
			}

			detailedDataBytes, err := json.Marshal(rewardDetailedData)
			if err != nil {
				log.Error().Msgf("serialize detailed data to json error: %+v, detailed data: %+v", err, rewardDetailedData)
				return nil
			}

			return &model.Application{
				Type:         model.ApplicationNewReward,
				Applicant:    user.Wallet,
				State:        model.ApplicationStateOpen,
				CreatedAt:    time.Now().In(internal.ProjectTimezone),
				UpdatedAt:    time.Now().In(internal.ProjectTimezone),
				DetailedType: appRcd.DetailedType,
				DetailedData: detailedDataBytes,
				EntityType:   newAppBundleReq.Entity,
				EntityId:     newAppBundleReq.EntityId,
				SeasonId:     seasonRecord.ID,
				Season:       seasonRecord,
			}
		})

		err = tx.Model(model.AppBundle{}).Create(&appBundle).Error
		if err != nil {
			return err
		}
		return tx.Model(model.AppBundleAuditLog{}).Create(&model.AppBundleAuditLog{
			AppBundleId: appBundle.ID,
			AppBundle:   appBundle,
			LogTs:       time.Now().In(internal.ProjectTimezone),
			Operation:   model.AuditActionNew,
			Operator:    user.Wallet,
			PreState:    "",
			PostState:   model.ApplicationStateOpen,
			ExtraData:   "",
		}).Error
	})

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusCreated, api.Success(nil))
}

func ApproveAppBundle(ctx *gin.Context) {

}

func RejectAppBundle(ctx *gin.Context) {

}
