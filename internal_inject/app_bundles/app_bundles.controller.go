package appbundles_inject

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type AppBundlesController struct {
	// inject

	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	AppBundlesService *AppBundlesService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var appBundles AppBundlesController

	err := inject.Populate(&appBundles, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	if fatherGroup != nil {
		fatherGroup.Group("/", middleware.AuthRequired).POST("/app_bundle_approve", appBundles.ApproveAppBundles)
		fatherGroup.Group("/", middleware.AuthRequired).POST("/app_bundle_reject", appBundles.RejectAppBundles)
	} else {
		appBundles.Gin.Group("/", middleware.AuthRequired).POST("/app_bundle_approve", appBundles.ApproveAppBundles)
		appBundles.Gin.Group("/", middleware.AuthRequired).POST("/app_bundle_reject", appBundles.RejectAppBundles)
	}

	var appBundlesAuthGroup *gin.RouterGroup

	if fatherGroup != nil {
		appBundlesAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/app_bundles")
	} else {
		appBundlesAuthGroup = appBundles.Gin.Group("/", middleware.AuthRequired).Group("/app_bundles")
	}

	appBundlesAuthGroup.GET("/available_projects_guilds", appBundles.ListAvailableProjectsAndGuilds)
	appBundlesAuthGroup.GET("/", appBundles.ListAppBundle)
	appBundlesAuthGroup.POST("/", appBundles.CreateAppBundle)

}

func (c *AppBundlesController) ListAvailableProjectsAndGuilds(ctx *gin.Context) {
	user, enforcer, _, _ := api.ForContext(ctx)

	ok, err := enforcer.HasRoleForUser(common.FormatUserWallet(user.Wallet), internal.RoleHall)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return
	}

	var guilds []*model.Guild
	var projects []*model.Project

	if ok {
		guilds, _, err = model.GuildModel.List(c.Db, nil)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list guilds error")))
			return
		}

		projects, _, err = model.ProjectModel.List(c.Db, "open,close_failed", nil, true)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list projects error")))
			return
		}
	} else {
		guilds, _, err = model.GuildModel.ListBySponsor(c.Db, common.FormatUserWallet(user.Wallet), nil)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list guilds error")))
			return
		}

		projects, _, err = model.ProjectModel.ListBySponsor(c.Db, common.FormatUserWallet(user.Wallet), "open", nil, false)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list projects error")))
			return
		}
	}

	var commonBudgetSources []*model.CommonBudgetSource
	err = c.Db.Model(&model.CommonBudgetSource{}).Find(&commonBudgetSources).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list common budget sources error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&ListAvailableProjectAndGuildResp{
		guilds,
		projects,
		commonBudgetSources,
	}))
}

func (c *AppBundlesController) ListAppBundle(ctx *gin.Context) {
	queryParams := model.ListAppBundleQueryParams{}
	if err := ctx.Bind(&queryParams); err != nil {
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("query params error: %+v", err)))
		return
	}

	appBundleRecords, total, err := model.QueryAppBundleRecords(c.Db, &queryParams)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query result error")))
		return
	}

	respRcds := lo.Map(appBundleRecords, func(jointAppBundleEntityRcd model.JointAppBundleEntityRslt, index int) AppBundleResponseRecord {
		assetSummary := make(map[string]decimal.Decimal)

		var appRcds []*model.Application
		err = c.Db.Where("bundle_id = ?", jointAppBundleEntityRcd.AppBundle.ID).
			Find(&appRcds).
			Error
		if err != nil {
			log.Error().Msgf("query application error: %+v", err)
			return AppBundleResponseRecord{}
		}

		// Summarize the assets in app bundle
		for _, appRcd := range appRcds {
			model.SetDefaultMapValue(assetSummary, appRcd.AssetName, decimal.Zero)
			assetSummary[appRcd.AssetName] = assetSummary[appRcd.AssetName].Add(appRcd.AssetAmount)
		}

		appIds := lo.Map(appRcds, func(appRcd *model.Application, index int) uint {
			return appRcd.ID
		})

		frontApplicationRecords, err := model.GenerateFrontendApplicationRecordsByIds(c.Db, appIds)
		if err != nil {
			log.Error().Msgf("query application error: %+v", err)
			return AppBundleResponseRecord{}
		}

		return AppBundleResponseRecord{
			ID:         jointAppBundleEntityRcd.AppBundle.ID,
			SeasonName: jointAppBundleEntityRcd.SeasonName,
			Records:    frontApplicationRecords,
			Entity: struct {
				Id   uint   `json:"id"`
				Name string `json:"name"`
				Type string `json:"type"`
			}{
				Id:   jointAppBundleEntityRcd.AppBundle.EntityId,
				Name: jointAppBundleEntityRcd.EntityName,
				Type: jointAppBundleEntityRcd.AppBundle.EntityType,
			},
			Applicant: jointAppBundleEntityRcd.AppBundle.Applicant,
			ApplyTime: jointAppBundleEntityRcd.AppBundle.CreatedAt,
			ApplyTs:   jointAppBundleEntityRcd.AppBundle.CreateTs,
			Comment:   jointAppBundleEntityRcd.AppBundle.Comment,
			State:     jointAppBundleEntityRcd.AppBundle.State,
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

func (c *AppBundlesController) CreateAppBundle(ctx *gin.Context) {
	var newAppBundleReq model.NewAppBundleRequest
	if err := ctx.BindJSON(&newAppBundleReq); err != nil {
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request error: %+v", err)))
		return
	}

	// Validate target user wallet
	for _, appRcd := range newAppBundleReq.Records {
		if targetUserWalletValidFlag := common.ValidateUserWallet(appRcd.TargetUserWallet); !targetUserWalletValidFlag {
			err := fmt.Errorf("invalid target user wallet: %s", appRcd.TargetUserWallet)
			sdk.LogUserSideError(ctx, err)
			ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
			return
		}
	}

	httpCode, reply := c.AppBundlesService.CreateAppBundle(ctx, &newAppBundleReq)

	ctx.JSON(httpCode, reply)
}

func (c *AppBundlesController) ApproveAppBundles(ctx *gin.Context) {
	log.Debug().Msg("has path ApproveAppBundles")

	httpCode, reply := c.AppBundlesService.UpdateAppBundleToNewState(ctx, model.ApplicationStateApproved)

	ctx.JSON(httpCode, reply)
}

func (c *AppBundlesController) RejectAppBundles(ctx *gin.Context) {
	httpCode, reply := c.AppBundlesService.UpdateAppBundleToNewState(ctx, model.ApplicationStateRejected)

	ctx.JSON(httpCode, reply)
}
