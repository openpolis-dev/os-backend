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

// ListVoteGates function lists all vote gates
//
//	@summary	lists all vote gates
//	@route		/v1/proposal_vote_gates [get]
//	@tags		proposal
//	@success	200	{object}	api.Reply{data=[]FrontendVoteGateResponse}
func ListVoteGates(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)
	var dbRecords []*model.ProposalVoteGate
	if err := db.Model(&model.ProposalVoteGate{}).Find(&dbRecords).Error; err != nil {
		log.Error().Msgf("fetch poll gates error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list poll gates error")))
		return
	}

	responseRecords := lo.Map(dbRecords, func(r *model.ProposalVoteGate, _ int) *FrontendVoteGateResponse {
		return &FrontendVoteGateResponse{
			ID:        r.ID,
			Name:      r.Name,
			TokenAddr: r.TokenAddress,
			TokenId:   r.TokenId,
			TokenType: r.TokenTypeName(),
			ChainType: r.ChainName(),
		}
	})

	ctx.JSON(http.StatusOK, api.Success(responseRecords))
}
