package datasrv_inject

import (
	"errors"
	"net/http"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
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

type DataSrvController struct {
	// Inject
	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	DataSrvService *DataSrvService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var dataSrv DataSrvController

	err := inject.Populate(&dataSrv, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var dataSrvGroup *gin.RouterGroup
	var dataSrvAuthGroup *gin.RouterGroup

	if fatherGroup != nil {
		dataSrvGroup = fatherGroup.Group("/data_srv")
		dataSrvAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/data_srv")
	} else {
		dataSrvGroup = dataSrv.Gin.Group("/data_srv")
		dataSrvAuthGroup = dataSrv.Gin.Group("/", middleware.AuthRequired).Group("/data_srv")
	}

	dataSrvGroup.GET("/aggr_scr", dataSrv.AggrScr)
	dataSrvAuthGroup.GET("/widget_data", dataSrv.WidgetData)
}

func (c *DataSrvController) AggrScr(ctx *gin.Context) {
	// Fetch current season data from database
	currentSeason, err := model.GetCurrentSeason(c.Db)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get current season error detail:"+err.Error())))
		return
	}
	log.Debug().Msgf("current season: %+v", currentSeason)

	mintResult, err := c.DataSrvService.CalcMintRewards(ctx, currentSeason)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("calc mint rewards error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&NodeCalcResponse{
		SeasonName:                   currentSeason.Name,
		SeasonTotalCreditWithoutMint: mintResult.TotalSeasonCreditWithoutMint.String(),
		SeasonTotalMintCredit:        mintResult.TotalMetaforoCredits.String(),
		TotalWalletCount:             len(mintResult.UserCredits),
		ActivateWalletCount:          mintResult.ActivateWalletCount,
		MintRewardConfirmed:          currentSeason.MintRewardConfirmed,
		SeedSnapshoted:               currentSeason.SeedSnapshotSaved,
		Records:                      mintResult.DetailRecords,
	}))
}

func (c *DataSrvController) WidgetData(ctx *gin.Context) {
	dataType := ctx.Query("type")
	if dataType == "" {
		err := errors.New("data type is required")
		log.Error().Err(err).Msg("missing data type")
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadGateway, api.BadRequest(err))
		return
	}

	user, enforcer, db, cfg := api.ForContext(ctx)
	allEntities, err := enforcer.HasRoleForUser(common.FormatUserWallet(user.Wallet), internal.RoleHall)
	if err != nil {
		log.Error().Err(err).Msg("casbin error")
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query user permission error detail:"+err.Error())))
		return
	}

	switch WidgetDataType(dataType) {
	case WidgetDataTypeProjectList:
		rcds, err := getEntityListResponse(db, "project", allEntities, user.Wallet)

		if err != nil {
			log.Error().Err(err).Msg("query project list error")
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query project list error detail:"+err.Error())))
			return
		}

		ctx.JSON(http.StatusOK, api.Success(rcds))
		return
	case WidgetDataTypeCommonProjectList:
		rcds, err := model.ProjectModel.GetCommonProjects(db)
		if err != nil {
			log.Error().Err(err).Msg("query project list error")
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query project list error detail:"+err.Error())))
			return
		}

		ctx.JSON(http.StatusOK, api.Success(rcds))
		return
	case WidgetDataTypeGuildList:
		rcds, err := getEntityListResponse(db, "guild", allEntities, user.Wallet)
		if err != nil {
			log.Error().Err(err).Msg("query guild list error")
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query guild list error detail:"+err.Error())))
			return
		}
		ctx.JSON(http.StatusOK, api.Success(rcds))
		return
	case WidgetDataTypeEntityList:
		projectRcds, err := getEntityListResponse(db, "project", allEntities, user.Wallet)
		projectRcds = lo.Map(projectRcds, func(rcd *WidgetDataResponse, _ int) *WidgetDataResponse {
			rcd.Type = "project"
			return rcd
		})

		if err != nil {
			log.Error().Err(err).Msg("query project list error")
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query project list error detail:"+err.Error())))
			return
		}

		guildRcds, err := getEntityListResponse(db, "guild", allEntities, user.Wallet)
		guildRcds = lo.Map(guildRcds, func(rcd *WidgetDataResponse, _ int) *WidgetDataResponse {
			rcd.Type = "guild"
			return rcd
		})
		if err != nil {
			log.Error().Err(err).Msg("query guild list error")
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query guild list error detail:"+err.Error())))
			return
		}
		ctx.JSON(http.StatusOK, api.Success(lo.Union(projectRcds, guildRcds)))
		return
	case WidgetDataTypeAssetForProposal:
		var assetResponse []*WidgetDataResponse
		for _, asset := range cfg.MetaforoData.ProposalAssets {
			assetResponse = append(assetResponse, &WidgetDataResponse{ID: asset.ID, Name: asset.Name})
		}
		ctx.JSON(http.StatusOK, api.Success(assetResponse))
		return
	case WidgetDataTypeAssetForApplication:
		var assetResponse []*WidgetDataResponse
		for _, asset := range cfg.MetaforoData.ApplicationAssets {
			assetResponse = append(assetResponse, &WidgetDataResponse{ID: asset.ID, Name: asset.Name})
		}
		ctx.JSON(http.StatusOK, api.Success(assetResponse))
		return
	case WidgetDataTypePassedProposals:
		rcds, err := c.DataSrvService.GetPassedProposals(user.Wallet, allEntities)
		if err != nil {
			log.Error().Err(err).Msg("query passed proposal error")
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query passed proposal error detail:"+err.Error())))
			return
		}
		ctx.JSON(http.StatusOK, api.Success(rcds))
		return
	case WidgetDataTypeCanBeVetoedProposals:
		rcds, err := c.DataSrvService.GetProposalsCanBeVetoed()
		if err != nil {
			log.Error().Err(err).Msg("query passed proposal error")
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query passed proposal error detail:"+err.Error())))
			return
		}
		ctx.JSON(http.StatusOK, api.Success(rcds))
		return
	default:
		err := errors.New("invalid data type")
		log.Error().Err(err).Msg("missing data type")
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadGateway, api.BadRequest(err))
		return
	}
}
