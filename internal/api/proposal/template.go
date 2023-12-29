package proposal

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
)

type TemplateResponse struct {
	ID     uint   `json:"id"`
	Name   string `json:"name"`
	Schema string `json:"schema"`
}

// ListTemplates list templates and return to frontend
//
//	@summary	list templates and return to frontend
//	@tags		proposals
//	@success	200	{object}	api.Reply{data=[]TemplateResponse}
//	@router		/proposal_tmpl/ [get]
func ListTemplates(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)
	var dbRcds []*TemplateResponse
	if err := db.Model(&model.ProposalTemplate{}).Find(&dbRcds).Error; err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(500, api.ServerError(errors.New("list templates failed")))
		return
	}

	respRcds := lo.Map(dbRcds, func(r *TemplateResponse, _ int) *TemplateResponse {
		return &TemplateResponse{
			ID:     r.ID,
			Name:   r.Name,
			Schema: r.Schema,
		}
	})

	ctx.JSON(200, api.Success(respRcds))
}
