package proposal

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
)

type VoterInfo struct {
	MetaforoUserId int    `json:"metaforo_user_id"`
	Wallet         string `json:"wallet"`
	Avatar         string `json:"avatar"`
}

// CastVote handles the casting of votes.
//
//	@summary	Cast a vote
//	@tags		Proposal
//	@param		id		query		number			true	"proposal ID"
//	@param		data	body		CastVoteData	true	"Vote data"
//	@success	200		{object}	api.Reply		"Success"
//	@router		/proposals/vote/:id [post]
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
//	@tags		Proposal
//	@param		id		query		number				true	"proposal ID"
//	@param		data	body		RevokeVoteData		true	"revoke vote data"
//	@success	200		{object}	api.Reply{data=nil}	"Success"
//	@router		/proposals/revoke_vote/:id [post]
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

// ShowVoteDetail returns vote detail for specified vote
//
//	@summary	revoke vote on existing metaforo vote
//	@tags		Proposal
//	@param		vote_option_id	path		number												true	"Vote ID"
//	@param		page			query		number												false	"page of the vote list"
//	@success	200				{object}	api.Reply{data=[]metaforo.UserPollRecordResponse}	"Success"
//	@router		/proposals/vote_detail/:vote_option_id [get]
func ShowVoteDetail(ctx *gin.Context) {
	voteIdStr := ctx.Param("vote_option_id")
	voteId, err := strconv.Atoi(voteIdStr)
	if err != nil {
		err := fmt.Errorf("parse request data error: %+v", err)
		log.Error().Msgf(err.Error())
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
		return
	}

	page := 1
	pageStr := ctx.Query("page")
	if pageStr != "" {
		page, err = strconv.Atoi(pageStr)
		if err != nil {
			err := fmt.Errorf("parse request data error: %+v", err)
			log.Error().Msgf(err.Error())
			sdk.LogUserSideError(ctx, err)
			ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
			return
		}
	}

	voterList, err := metaforo.GetVoterList(internal.MetaforoGroupName, voteId, page)
	if err != nil {
		log.Error().Msgf("get vote list error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("get vote list error")))
		return
	}

	db := api.ForContextOnlyDB(ctx)
	db.Raw(QueryMetaforoUserWithOsUserBaseSQL)
	// TODO: Find OS user with voter user id
	OsVoterRecords := make([]*VoterInfo, 0)
	for _, voterInfo := range voterList {
		osVoterInfo := &VoterInfo{
			MetaforoUserId: voterInfo.UserId,
		}
		OsVoterRecords = append(OsVoterRecords, osVoterInfo)
	}

	ctx.JSON(http.StatusOK, api.Success(OsVoterRecords))
}
