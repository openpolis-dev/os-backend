package proposal

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
)

type TemplateResponse struct {
	ID            uint                 `json:"id"`
	Name          string               `json:"name"`
	ScreenshotUri string               `json:"screenshot_uri"`
	ContentSchema string               `json:"schema"`
	CategoryName  string               `json:"-"`
	HasPerm       bool                 `json:"has_perm"`
	Components    []*ComponentResponse `json:"components"`
}

// ListTemplates list templates and return to frontend
//
//	@summary	list templates and return to frontend
//	@tags		Proposal
//	@success	200	{object}	api.Reply{data=[]TemplateResponse}
//	@router		/proposals/proposal_tmpl/list [get]
func ListTemplates(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)
	var dbRcds []*model.ProposalTemplate
	if err := db.Model(&model.ProposalTemplate{}).Preload("Components").Find(&dbRcds).Error; err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list templates failed")))
		return
	}

	respRcds := lo.Map(dbRcds, func(r *model.ProposalTemplate, _ int) *TemplateResponse {
		return &TemplateResponse{
			ID:            r.ID,
			Name:          r.Name,
			ContentSchema: r.ContentSchema,
			ScreenshotUri: r.ScreenshotUri,
			Components: lo.Map(r.Components, func(c *model.ProposalComponent, _ int) *ComponentResponse {
				return &ComponentResponse{
					ID:            c.ID,
					Name:          c.Name,
					Schema:        c.Schema,
					ScreenshotUri: c.ScreenshotUri,
				}
			}),
		}
	})

	ctx.JSON(200, api.Success(respRcds))
}

// ListTemplatesWithPerm list templates in leveled struct and add permission check
//
//	@summary	list templates and return to frontend
//	@tags		Proposal
//	@success	200	{object}	api.Reply{data=[]TemplateResponse}
//	@router		/proposals/proposal_tmpl/list_with_perm [get]
func ListTemplatesWithPerm(ctx *gin.Context) {
	user, _, db, _ := api.ForContext(ctx)

	sppClient := sdk.GetSppClient()
	userSeepassData, err := api.GetCachedSeepassData(sppClient, user.Wallet, false)

	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("get user %+v seepass data error: %+v", user, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get user Seepass data error")))
		return
	}

	var dbRcds []*model.ProposalTemplate
	if err := db.Model(&model.ProposalTemplate{}).
		Preload("ProposalVoteGates").
		Preload("ProposalCategory").
		Preload("Components").Find(&dbRcds).Error; err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list templates failed")))
		return
	}

	tmplRecords := lo.Map(dbRcds, func(r *model.ProposalTemplate, _ int) *TemplateResponse {
		// TODO: Validate permissions of vote gate
		permArray := lo.Map(r.ProposalVoteGates, func(r *model.ProposalVoteGate, _ int) bool {
			return IsUserMetVoteGate(userSeepassData, r)
		})

		hasPerm := lo.Reduce(permArray, func(rslt bool, r bool, _ int) bool {
			return rslt && r
		}, true)

		return &TemplateResponse{
			ID:            r.ID,
			Name:          r.Name,
			ContentSchema: r.ContentSchema,
			ScreenshotUri: r.ScreenshotUri,
			CategoryName:  r.ProposalCategory.Name,
			HasPerm:       hasPerm,
			Components: lo.Map(r.Components, func(c *model.ProposalComponent, _ int) *ComponentResponse {
				return &ComponentResponse{
					ID:            c.ID,
					Name:          c.Name,
					Schema:        c.Schema,
					ScreenshotUri: c.ScreenshotUri,
				}
			}),
		}
	})

	respRcds := lo.GroupBy(tmplRecords, func(r *TemplateResponse) string {
		return r.CategoryName
	})

	ctx.JSON(200, api.Success(respRcds))
}
