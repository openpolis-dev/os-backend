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

// ListCategoriesWithPerm function lists all proposal categories with permissions
//
//	@summary	list all proposal categories with permission field
//	@router		/proposal_categories/list_with_perm [get]
//	@tags		Proposal
//	@success	200	{object}	api.Reply{data=[]proposal.FrontendProposalCategory}
func ListCategoriesWithPerm(ctx *gin.Context) {
	user, _, db, _ := api.ForContext(ctx)

	sppClient := sdk.GetSppClient()
	userSeepassData, err := api.GetCachedSeepassData(sppClient, user.Wallet, false)

	var proposalCategories []*model.ProposalCategory
	err = db.Model(&model.ProposalCategory{}).
		Joins("ProposalVoteGate").
		Where(model.ProposalCategory{IsActive: true}).Find(&proposalCategories).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("get proposal categories error: %+v", err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal categories error")))
		return
	}

	categoryResp := lo.Map(proposalCategories, func(r *model.ProposalCategory, index int) *FrontendProposalCategory {
		return &FrontendProposalCategory{
			ID:         r.ID,
			ParentID:   r.ParentID,
			Name:       r.Name,
			MetaforoId: r.MetaforoId,
			HasPerm:    IsUserMetVoteGate(userSeepassData, r.ProposalVoteGate),
		}
	})

	ctx.JSON(http.StatusOK, api.Reply{
		Data: categoryResp,
	})

}

// ListAllCategories return all categories data, open to public
//
//	@summary	list all proposal categories
//	@router		/proposal_categories/list [get]
//	@tags		Proposal
//	@success	200	{object}	api.Reply{data=[]proposal.FrontendProposalCategory}
func ListAllCategories(ctx *gin.Context) {
	var err error
	db := api.ForContextOnlyDB(ctx)
	var proposalCategories []*model.ProposalCategory
	err = db.Model(&model.ProposalCategory{}).Where(model.ProposalCategory{IsActive: true}).Find(&proposalCategories).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("get proposal categories error: %+v", err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal categories error")))
		return
	}

	categoryResp := lo.Map(proposalCategories, func(r *model.ProposalCategory, index int) *FrontendProposalCategory {
		return &FrontendProposalCategory{
			ID:         r.ID,
			ParentID:   r.ParentID,
			Name:       r.Name,
			MetaforoId: r.MetaforoId,
		}
	})

	ctx.JSON(http.StatusOK, api.Reply{
		Data: categoryResp,
	})

}
