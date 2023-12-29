package proposal

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
)

// CastVote handles the casting of votes.
//
//	@summary	Cast a vote
//	@tags		proposals
//	@param		id		query		number			true	"proposal ID"
//	@param		data	body		CastVoteData	true	"Vote data"
//	@success	200		{object}	api.Reply		"Success"
//	@router		/vote/:id [post]
func CastVote(ctx *gin.Context) {
	reqData := CastVoteData{}
	if err := ctx.BindJSON(&reqData); err != nil {
		log.Error().Msgf("parse request data error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
		return
	}

	if err := metaforo.CastVote(
		reqData.MetaforoAccessToken,
		internal.MetaforoGroupName,
		reqData.MetaforoVoteId,
		reqData.MetaforoVoteOptions,
	); err != nil {
		log.Error().Msgf("cast vote error error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("cast vote error")))
		return

	}
	ctx.JSON(http.StatusOK, api.Success(nil))
}

// RevokeVote revoke vote on existing metaforo vote
//
//	@summary	revoke vote on existing metaforo vote
//	@tags		proposals
//	@param		id		query		number				true	"proposal ID"
//	@param		data	body		RevokeVoteData		true	"revoke vote data"
//	@success	200		{object}	api.Reply{data=nil}	"Success"
//	@router		/revoke_vote/:id [post]
func RevokeVote(ctx *gin.Context) {
	reqData := RevokeVoteData{}
	if err := ctx.BindJSON(&reqData); err != nil {
		log.Error().Msgf("parse request data error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
		return
	}

	if err := metaforo.RevokeVote(
		reqData.MetaforoAccessToken,
		internal.MetaforoGroupName,
		reqData.MetaforoVoteId,
	); err != nil {
		log.Error().Msgf("revoke vote error error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("revoke vote error")))
		return

	}
	ctx.JSON(http.StatusOK, api.Success(nil))
}
