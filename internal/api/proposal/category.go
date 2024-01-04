package proposal

import (
	"errors"
	"net/http"
	"strings"

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
//	@tags		Proposal
//	@success	200	{object}	api.Reply{data=[]proposal.FrontendProposalCategory}
func ListCategories(ctx *gin.Context) {
	user, _, db, _ := api.ForContext(ctx)

	sppClient := sdk.GetSppClient()
	userSeepassData, err := sppClient.GetSeepassData(user.Wallet)

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
		userHasPerm := false
		if userSeepassData != nil {
			if r.ProposalVoteGateId != 0 {
				for _, sbtInfo := range userSeepassData.Sbt {
					if strings.EqualFold(sbtInfo.ContractAddr, r.ProposalVoteGate.TokenAddress) {
						if r.ProposalVoteGate.TokenTypeName() == "ERC1155" {
							userHasPerm = strings.EqualFold(r.ProposalVoteGate.TokenId, sbtInfo.TokenId)
						} else {
							userHasPerm = true
						}
					}
				}
			} else {
				// No nft gate set, every one can create proposal
				userHasPerm = true
			}
		}
		return &FrontendProposalCategory{
			ID:         r.ID,
			ParentID:   r.ParentID,
			Name:       r.Name,
			MetaforoId: r.MetaforoId,
			HasPerm:    userHasPerm,
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
