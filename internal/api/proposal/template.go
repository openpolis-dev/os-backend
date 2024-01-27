package proposal

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type TemplateResponse struct {
	ID               uint                 `json:"id"`
	Name             string               `json:"name"`
	ScreenshotUri    string               `json:"screenshot_uri"`
	ContentSchema    string               `json:"schema"`
	CategoryName     string               `json:"-"`
	CategoryId       uint                 `json:"-"`
	HasPermToUse     bool                 `json:"has_perm_to_use"`
	RuleDescription  string               `json:"rule_description"`
	IsInstantVote    bool                 `json:"is_instant_vote"`
	IsClosingProject bool                 `json:"is_closing_project"`
	VoteType         int                  `json:"vote_type"`
	Components       []*ComponentResponse `json:"components"`
}

type TmplWithCategoryNameRecord struct {
	CategoryId   uint                `json:"category_id"`
	CategoryName string              `json:"category_name"`
	Templates    []*TemplateResponse `json:"templates"`
}

type updateTmplRequest struct {
	Name          string   `json:"name"`
	CategoryId    uint     `json:"category_id"`
	Schema        string   `json:"schema"`
	ScreenshotUri string   `json:"screenshot_uri"`
	Components    []string `json:"components"`
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
			ID:               r.ID,
			Name:             r.Name,
			ContentSchema:    r.ContentSchema,
			ScreenshotUri:    r.ScreenshotUri,
			RuleDescription:  r.RuleDesc,
			IsInstantVote:    r.PublicitySecond == 0,
			IsClosingProject: strings.Contains(r.Name, internal.ClosingProjectTemplateMagicWord),
			VoteType:         r.VoteType,
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
		Preload("Components").Order("proposal_category_id").Find(&dbRcds).Error; err != nil {
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
			ID:               r.ID,
			Name:             r.Name,
			ContentSchema:    r.ContentSchema,
			ScreenshotUri:    r.ScreenshotUri,
			CategoryName:     r.ProposalCategory.Name,
			CategoryId:       r.ProposalCategoryID,
			HasPermToUse:     hasPermToUse,
			RuleDescription:  r.RuleDesc,
			IsInstantVote:    r.PublicitySecond == 0,
			IsClosingProject: strings.Contains(r.Name, internal.ClosingProjectTemplateMagicWord),
			VoteType:         r.VoteType,
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

	respRcdMap := lo.GroupBy(tmplRecords, func(r *TemplateResponse) lo.Tuple2[uint, string] {
		return lo.T2[uint, string](r.CategoryId, r.CategoryName)
	})

	respRcds := lo.MapToSlice(respRcdMap, func(categoryIdName lo.Tuple2[uint, string], tmplRcds []*TemplateResponse) *TmplWithCategoryNameRecord {
		categoryId, categoryName := lo.Unpack2(categoryIdName)
		return &TmplWithCategoryNameRecord{
			CategoryId:   categoryId,
			CategoryName: categoryName,
			Templates:    tmplRcds,
		}
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

	newTmplRcd := model.ProposalTemplate{
		Name:               reqData.Name,
		ContentSchema:      reqData.Schema,
		ScreenshotUri:      reqData.ScreenshotUri,
		ProposalCategoryID: reqData.CategoryId,
	}

	var components []*model.ProposalComponent
	for _, compName := range reqData.Components {
		compRcd := model.ProposalComponent{Name: compName}
		if err := db.Model(&compRcd).Where("name = ?", compName).First(&compRcd).Error; err != nil {
			log.Error().Msgf("parse request data error: %+v", err)
			sdk.LogUserSideError(ctx, err)
			ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
			return
		}

		components = append(components, &compRcd)
	}

	newTmplRcd.Components = components

	var tmplRcd model.ProposalTemplate
	if err := db.Model(&tmplRcd).Where("name = ?", reqData.Name).First(&tmplRcd).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = db.Model(&newTmplRcd).Create(&newTmplRcd).Error
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
		// Update existing data
		newTmplRcd.ID = tmplRcd.ID
		// Clear existing m2m components
		err := db.Model(&tmplRcd).Association("Components").Clear()
		if err != nil {
			log.Error().Msgf("update proposal template error: %+v", err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
		err = db.Save(&newTmplRcd).Error
		if err != nil {
			log.Error().Msgf("update proposal template error: %+v", err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
	}

	ctx.JSON(200, api.Success(newTmplRcd))
}
