package proposal

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type TemplateResponse struct {
	ID              uint                 `json:"id"`
	Name            string               `json:"name"`
	ScreenshotUri   string               `json:"screenshot_uri"`
	ContentSchema   string               `json:"schema"`
	CategoryName    string               `json:"-"`
	HasPermToUse    bool                 `json:"has_perm_to_use"`
	RuleDescription string               `json:"rule_description"`
	IsInstantVote   bool                 `json:"is_instant_vote"`
	Components      []*ComponentResponse `json:"components"`
}

type updateTmplRequest struct {
	Name          string `json:"name"`
	Schema        string `json:"schema"`
	ScreenshotUri string `json:"screenshot_uri"`
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
		Preload("UseTemplateGates").
		Preload("ProposalCategory").
		Preload("Components").Find(&dbRcds).Error; err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list templates failed")))
		return
	}

	tmplRecords := lo.Map(dbRcds, func(r *model.ProposalTemplate, _ int) *TemplateResponse {
		// TODO: Validate permissions of vote gate
		permArray := lo.Map(r.UseTemplateGates, func(r *model.ProposalVoteGate, _ int) bool {
			return IsUserMetVoteGate(userSeepassData, r)
		})

		hasPermToUse := lo.Reduce(permArray, func(rslt bool, r bool, _ int) bool {
			return rslt && r
		}, true)

		return &TemplateResponse{
			ID:            r.ID,
			Name:          r.Name,
			ContentSchema: r.ContentSchema,
			ScreenshotUri: r.ScreenshotUri,
			CategoryName:  r.ProposalCategory.Name,
			HasPermToUse:  hasPermToUse,
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

// UpdateTemplate requires to be invoked with Admin perm
func UpdateTemplate(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)
	reqData := updateTmplRequest{}
	if err := ctx.BindJSON(&reqData); err != nil {
		log.Error().Msgf("parse request data error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
		return
	}

	tmplRcd := model.ProposalTemplate{
		Name:          reqData.Name,
		ContentSchema: reqData.Schema,
		ScreenshotUri: reqData.ScreenshotUri,
	}

	if err := db.Model(&tmplRcd).Where("name = ?", reqData.Name).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = db.Model(&tmplRcd).Create(&tmplRcd).Error
			if err != nil {
				log.Error().Msgf("create proposal template error: %+v", err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
				return
			}
		} else {
			log.Error().Msgf("create proposal template error: %+v", err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
	} else {
		err = db.Save(&tmplRcd).Error
		if err != nil {
			log.Error().Msgf("update proposal template error: %+v", err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
	}

	ctx.JSON(200, api.Success(tmplRcd))
}
