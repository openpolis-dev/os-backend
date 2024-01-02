package proposal

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
)

type TemplateResponse struct {
	ID            uint                 `json:"id"`
	Name          string               `json:"name"`
	ContentSchema string               `json:"schema"`
	Components    []*ComponentResponse `json:"components"`
}

// ListTemplates list templates and return to frontend
//
//	@summary	list templates and return to frontend
//	@tags		proposals
//	@success	200	{object}	api.Reply{data=[]TemplateResponse}
//	@router		/proposal_tmpl/ [get]
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
			Components: lo.Map(r.Components, func(c *model.ProposalComponent, _ int) *ComponentResponse {
				return &ComponentResponse{
					ID:     c.ID,
					Name:   c.Name,
					Schema: c.Schema,
				}
			}),
		}
	})

	ctx.JSON(200, api.Success(respRcds))
}
