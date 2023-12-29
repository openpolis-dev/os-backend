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

// ListCategories function lists all proposal categories
//
//	@summary	list all proposal categories
//	@router		/proposal_categories [get]
//	@tags		proposal
//	@success	200	{object}	api.Reply{data=[]proposal.FrontendProposalCategory}
func ListCategories(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)
	proposalCategories, err := model.QueryRows[model.ProposalCategory](
		db.Model(&model.ProposalCategory{}).Where(model.ProposalCategory{IsActive: true}),
		nil)

	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("get proposal categories error: %+v", err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal categories error")))
		return
	}

	categoryResp := lo.Map(proposalCategories, func(item *model.ProposalCategory, index int) *FrontendProposalCategory {
		return &FrontendProposalCategory{
			ID:         item.ID,
			ParentID:   item.ParentID,
			Name:       item.Name,
			MetaforoId: item.MetaforoId,
		}
	})

	ctx.JSON(http.StatusOK, api.Reply{
		Data: categoryResp,
	})

}

// UpdateCategories function updates category and sync to Metaforo
func UpdateCategories(ctx *gin.Context) {
	req := UpdateProposalCategoryReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		sdk.LogUserSideError(ctx, err)
		log.Error().Msgf("UpdateProposalCategory: bind json error: %+v", err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	// TODO: Find record in DB with ID, and update parentID, name, metaforoId
	// TODO: Load meatforoID from parent record (if have) and sync name and hierarchical relationship to Metaforo
}

// SyncFromMetaforo is used to sync metaforo categories to local DB
// This function first queries ProposalCategory table by metaforoID,
// and update name and hierarchical relationship if found, or create new record with returned name if metaforo ID is not found
// Invoking this API should have hall permission, and should raise error if record with same name found
func SyncFromMetaforo(ctx *gin.Context) {
	//user, _, db, _ := api.ForContext(ctx)
}
