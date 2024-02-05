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

const listTemplateWithPermSQL = `select
pt.id,pt.name,pt.content_schema,pt.screenshot_uri,
pc.name as category_name,
pt.display_index as display_index,
pc.display_index as category_display_index,
pt.proposal_category_id as category_id,
pt.rule_desc as rule_description,
pt.publicity_second = 0 as is_instant_vote,
pt.type = ? as is_closing_project,
pt.vote_type
from proposal_templates pt
left join proposal_categories pc on pt.proposal_category_id =pc.id
where pt.is_hidden=false
order by pc.display_index, pt.display_index`

type TemplateResponse struct {
	ID                   uint   `json:"id"`
	Name                 string `json:"name"`
	DisplayIndex         int    `json:"display_index"`
	ScreenshotUri        string `json:"screenshot_uri"`
	ContentSchema        string `json:"schema"`
	CategoryName         string `json:"-"`
	CategoryId           uint   `json:"-"`
	CategoryDisplayIndex uint   `json:"category_display_index"`
	HasPermToUse         bool   `json:"has_perm_to_use"`
	RuleDescription      string `json:"rule_description"`
	IsInstantVote        bool   `json:"is_instant_vote"`
	IsClosingProject     bool   `json:"is_closing_project"`
	VoteType             int    `json:"vote_type"`
}

type TemplateResponseWithComponents struct {
	TemplateResponse
	Components []*ComponentResponse `json:"components"`
}

type TmplWithCategoryNameRecord struct {
	CategoryId           uint                              `json:"category_id"`
	CategoryDisplayIndex uint                              `json:"category_display_index"`
	CategoryName         string                            `json:"category_name"`
	Templates            []*TemplateResponseWithComponents `json:"templates"`
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

	respRcds := lo.Map(dbRcds, func(r *model.ProposalTemplate, _ int) *TemplateResponseWithComponents {
		tmplRsp := TemplateResponse{
			ID:               r.ID,
			Name:             r.Name,
			DisplayIndex:     r.DisplayIndex,
			ContentSchema:    r.ContentSchema,
			ScreenshotUri:    r.ScreenshotUri,
			RuleDescription:  r.RuleDesc,
			IsInstantVote:    r.PublicitySecond == 0,
			IsClosingProject: r.Type == model.ProposalTemplateTypeCloseProject,
			VoteType:         r.VoteType,
		}
		return &TemplateResponseWithComponents{
			TemplateResponse: tmplRsp,
			Components:       getTemplateComponents(r),
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

	var rcds []*TemplateResponse
	err = db.Raw(listTemplateWithPermSQL, model.ProposalTemplateTypeCloseProject).Scan(&rcds).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list templates failed")))
		return
	}

	tmplRecords := lo.Map(rcds, func(r *TemplateResponse, _ int) *TemplateResponseWithComponents {
		var tmplDbRcd model.ProposalTemplate
		err = db.Preload("Components").Find(&tmplDbRcd, r.ID).Error
		if err != nil {
			log.Warn().Msgf("try to fetch template record  %d error: %+v", r.ID, err)
			return nil
		}

		permArray := lo.Map(tmplDbRcd.UseTemplateGates, func(r *model.ProposalVoteGate, _ int) bool {
			return IsUserMetVoteGate(userSeepassData, r)
		})

		r.HasPermToUse = lo.Reduce(permArray, func(rslt bool, r bool, _ int) bool {
			return rslt && r
		}, true)

		components := getTemplateComponents(&tmplDbRcd)

		return &TemplateResponseWithComponents{
			TemplateResponse: *r,
			Components:       components,
		}
	})

	respRcdMap := lo.GroupBy(tmplRecords, func(r *TemplateResponseWithComponents) lo.Tuple3[uint, uint, string] {
		return lo.T3[uint, uint, string](r.CategoryId, r.CategoryDisplayIndex, r.CategoryName)
	})

	respRcds := lo.MapToSlice(respRcdMap, func(categoryIdName lo.Tuple3[uint, uint, string], tmplRcds []*TemplateResponseWithComponents) *TmplWithCategoryNameRecord {
		categoryId, categoryDisplayIndex, categoryName := lo.Unpack3(categoryIdName)
		return &TmplWithCategoryNameRecord{
			CategoryId:           categoryId,
			CategoryDisplayIndex: categoryDisplayIndex,
			CategoryName:         categoryName,
			Templates:            tmplRcds,
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
		err = db.Updates(&newTmplRcd).Error
		if err != nil {
			log.Error().Msgf("update proposal template error: %+v", err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
	}

	ctx.JSON(200, api.Success(newTmplRcd))
}

func getTemplateComponents(tmplDbRcd *model.ProposalTemplate) []*ComponentResponse {
	compNameMapping := lo.SliceToMap(tmplDbRcd.Components, func(c *model.ProposalComponent) (string, *model.ProposalComponent) {
		return c.Name, c
	})
	var components []*ComponentResponse
	if tmplDbRcd.ComponentNameList != nil {
		for _, name := range tmplDbRcd.ComponentNameList {
			if comp, ok := compNameMapping[name]; ok {
				components = append(components, &ComponentResponse{
					ID:            comp.ID,
					Name:          comp.Name,
					Schema:        comp.Schema,
					ScreenshotUri: comp.Schema,
				})
			}
		}
	} else {
		components = lo.Map(tmplDbRcd.Components, func(c *model.ProposalComponent, _ int) *ComponentResponse {
			return &ComponentResponse{
				ID:            c.ID,
				Name:          c.Name,
				Schema:        c.Schema,
				ScreenshotUri: c.ScreenshotUri,
			}
		})
	}

	return components
}
