package data_srv

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type WidgetDataResponse struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// TODO: Currently the data service is handled by RESTful API and query params, will be migrate to GraphQL in future

// WidgetData returns data required by widget
//
//	@summary	returns data required by widget.
//	@tags		DataService
//	@router		/data_srv/widget_data [get]
//	@param		type	query		string	true	"data type"	Enum(project_list guild_list entity_list asset_type)
//	@success	200		{object}	api.Reply{data=[]WidgetDataResponse}
func WidgetData(ctx *gin.Context) {
	dataType := ctx.Query("type")
	if dataType == "" {
		err := errors.New("data type is required")
		log.Error().Err(err).Msg("missing data type")
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadGateway, api.BadRequest(err))
		return
	}

	user, enforcer, db, _ := api.ForContext(ctx)
	allEntities, err := enforcer.HasRoleForUser(common.FormatUserWallet(user.Wallet), internal.RoleHall)
	log.Error().Msgf("TTT: all entities: %+v", allEntities)
	if err != nil {
		log.Error().Err(err).Msg("casbin error")
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query user permission error")))
		return
	}

	switch dataType {
	case "project_list":
		rcds, err := getEntityListResponse(db, "project", allEntities, user.Wallet)

		if err != nil {
			log.Error().Err(err).Msg("query project list error")
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query project list error")))
			return
		}

		ctx.JSON(http.StatusOK, api.Success(rcds))
		return
	case "guild_list":
		rcds, err := getEntityListResponse(db, "guild", allEntities, user.Wallet)
		if err != nil {
			log.Error().Err(err).Msg("query guild list error")
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query guild list error")))
			return
		}
		ctx.JSON(http.StatusOK, api.Success(rcds))
		return
	case "entity_list":
		projectRcds, err := getEntityListResponse(db, "project", allEntities, user.Wallet)

		if err != nil {
			log.Error().Err(err).Msg("query project list error")
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query project list error")))
			return
		}

		guildRcds, err := getEntityListResponse(db, "guild", allEntities, user.Wallet)
		if err != nil {
			log.Error().Err(err).Msg("query guild list error")
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query guild list error")))
			return
		}
		ctx.JSON(http.StatusOK, api.Success(lo.Union(projectRcds, guildRcds)))
		return
	case "asset_type":
		ctx.JSON(http.StatusOK, api.Success([]*WidgetDataResponse{
			{internal.AssetTypeScrId, internal.AssetTypeScrName}, {internal.AssetTypeUsdtId, internal.AssetTypeUsdtName},
		}))
		return
	default:
		err := errors.New("invalid data type")
		log.Error().Err(err).Msg("missing data type")
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadGateway, api.BadRequest(err))
		return
	}
}

func getEntityListResponse(db *gorm.DB, entityType string, allRecords bool, userWallet string) ([]*WidgetDataResponse, error) {
	var projects []*model.Project
	var guilds []*model.Guild
	var err error

	switch entityType {
	case "project":
		if allRecords {
			projects, _, err = model.ProjectModel.List(db, "open", nil, false)
		} else {
			projects, _, err = model.ProjectModel.ListBySponsor(db, common.FormatUserWallet(userWallet), "open", nil, false)
		}
		if err != nil {
			return nil, err
		}
		return lo.Map(projects, func(g *model.Project, _ int) *WidgetDataResponse {
			return &WidgetDataResponse{
				ID:   g.ID,
				Name: g.Name,
			}
		}), nil
	case "guild":
		if allRecords {
			guilds, _, err = model.GuildModel.List(db, nil)
		} else {
			guilds, _, err = model.GuildModel.ListBySponsor(db, common.FormatUserWallet(userWallet), nil)
		}
		return lo.Map(guilds, func(g *model.Guild, _ int) *WidgetDataResponse {
			return &WidgetDataResponse{
				ID:   g.ID,
				Name: g.Name,
			}
		}), nil
	default:
		return nil, errors.New("invalid entity type")
	}
}
