package data_srv

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/api/proposal"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type WidgetDataResponse struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`

	// This field is used for entity_list widget
	Type string `json:"type,omitempty"`

	// Those two fields are used for associating proposal widget
	CreateTs             int64  `json:"create_ts,omitempty"`
	ProposalCategoryName string `json:"proposal_category_name,omitempty"`
	ProposalState        string `json:"proposal_state,omitempty"`
	Applicant            string `json:"applicant,omitempty"`
	ApplicantAvatar      string `json:"applicant_avatar,omitempty"`
}

type WidgetDataType string

const (
	WidgetDataTypeProjectList          WidgetDataType = "project_list"
	WidgetDataTypeCommonProjectList                   = "common_project_list"
	WidgetDataTypeGuildList                           = "guild_list"
	WidgetDataTypeEntityList                          = "entity_list"
	WidgetDataTypeAssetForProposal                    = "asset_type_proposal"
	WidgetDataTypeAssetForApplication                 = "asset_type_app"
	WidgetDataTypePassedProposals                     = "passed_proposals"
	WidgetDataTypeCanBeVetoedProposals                = "can_be_vetoed_proposals"
)

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

	user, enforcer, db, cfg := api.ForContext(ctx)
	allEntities, err := enforcer.HasRoleForUser(common.FormatUserWallet(user.Wallet), internal.RoleHall)
	if err != nil {
		log.Error().Err(err).Msg("casbin error")
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query user permission error")))
		return
	}

	switch WidgetDataType(dataType) {
	case WidgetDataTypeProjectList:
		rcds, err := getEntityListResponse(db, "project", allEntities, user.Wallet)

		if err != nil {
			log.Error().Err(err).Msg("query project list error")
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query project list error")))
			return
		}

		ctx.JSON(http.StatusOK, api.Success(rcds))
		return
	case WidgetDataTypeCommonProjectList:
		rcds, err := model.ProjectModel.GetCommonProjectsOwnedByUser(db, user.Wallet)
		if err != nil {
			log.Error().Err(err).Msg("query project list error")
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query project list error")))
			return
		}

		ctx.JSON(http.StatusOK, api.Success(rcds))
		return
	case WidgetDataTypeGuildList:
		rcds, err := getEntityListResponse(db, "guild", allEntities, user.Wallet)
		if err != nil {
			log.Error().Err(err).Msg("query guild list error")
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query guild list error")))
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
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query project list error")))
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
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query guild list error")))
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
		rcds, err := getPassedProposals(db, user.Wallet, allEntities)
		if err != nil {
			log.Error().Err(err).Msg("query passed proposal error")
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query passed proposal error")))
			return
		}
		ctx.JSON(http.StatusOK, api.Success(rcds))
		return
	case WidgetDataTypeCanBeVetoedProposals:
		rcds, err := getProposalsCanBeVetoed(db)
		if err != nil {
			log.Error().Err(err).Msg("query passed proposal error")
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query passed proposal error")))
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

func getPassedProposals(db *gorm.DB, userWallet string, allRecords bool) ([]*WidgetDataResponse, error) {
	var rcds []*proposal.FrontendProposalListRecord
	querySql := fmt.Sprintf("%s WHERE state = %d", proposal.ListProposalsSQL, model.ProposalStateVotePassed)
	if !allRecords {
		querySql += fmt.Sprintf(" AND applicant = '%s'", common.FormatUserWallet(userWallet))
	}
	querySql += fmt.Sprintf(" ORDER BY id ASC")

	err := db.Raw(querySql).Find(&rcds).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Err(err).Msgf("no proposal found for user %s", userWallet)
			return []*WidgetDataResponse{}, nil
		} else {
			log.Error().Err(err).Msg("query proposal list error")
			return nil, err
		}
	}

	return convertFrontendEndProposalListRecordToWidgetDataResponse(rcds), nil
}

func getProposalsCanBeVetoed(db *gorm.DB) ([]*WidgetDataResponse, error) {
	var rcds []*proposal.FrontendProposalListRecord
	querySql := fmt.Sprintf("%s WHERE state IN (%d, %d) and p.can_be_vetoed = true",
		proposal.ListProposalsSQL,
		model.ProposalStateVoting,
		model.ProposalStatePendingExecution,
	)
	querySql += fmt.Sprintf(" ORDER BY create_ts DESC")

	err := db.Raw(querySql).Find(&rcds).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Err(err).Msgf("no proposal can be vetoed")
			return []*WidgetDataResponse{}, nil
		} else {
			log.Error().Err(err).Msg("query proposal list error")
			return nil, err
		}
	}
	return convertFrontendEndProposalListRecordToWidgetDataResponse(rcds), nil
}

func convertFrontendEndProposalListRecordToWidgetDataResponse(proposalRcds []*proposal.FrontendProposalListRecord) []*WidgetDataResponse {
	return lo.Map(proposalRcds, func(r *proposal.FrontendProposalListRecord, _ int) *WidgetDataResponse {
		if r.Sip != 0 {
			r.Title = fmt.Sprintf("SIP-%d: %s", r.Sip, r.Title)
		}
		return &WidgetDataResponse{
			ID:                   r.ID,
			Name:                 r.Title,
			ProposalCategoryName: r.CategoryName,
			ProposalState:        model.ProposalStateName[r.StateId],
			Applicant:            r.Applicant,
			ApplicantAvatar:      r.ApplicantAvatar,
			CreateTs:             r.CreateTs,
		}
	})
}
