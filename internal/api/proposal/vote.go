package proposal

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"gorm.io/gorm"
)

type VoterInfo struct {
	MetaforoUserId int    `json:"metaforo_user_id"`
	Wallet         string `json:"wallet"`
	Avatar         string `json:"avatar"`
}

// CheckVotePermission checks whether current user can vote on this proposal
//
//	@summary	Check whether user has permission to vote in thread
//	@tags		Proposal
//	@router		/proposals/can_vote/:id [post]
//	@param		id	query		number		true	"proposal ID"
//	@success	200	{object}	api.Reply	"Success"
//	@success	401	{object}	api.Reply	"Forbidden"
func CheckVotePermission(ctx *gin.Context) {
	user, _, db, _ := api.ForContext(ctx)

	proposalIdString := ctx.Param("id")
	userHasVotePermissionOnThread, err := canUserVoteOnThread(db, user.Wallet, proposalIdString)
	if err != nil {
		log.Error().Msgf("check user vote permission error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("check user permission error")))
		return
	}
	ctx.JSON(http.StatusOK, api.Success(userHasVotePermissionOnThread))
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
	user, _, db, cfg := api.ForContext(ctx)

	proposalIdString := ctx.Param("id")
	userHasVotePermissionOnThread, err := canUserVoteOnThread(db, user.Wallet, proposalIdString)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("check user vote permission error: %+v", err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("check user permission error")))
		return
	}

	if !userHasVotePermissionOnThread {
		err := fmt.Errorf("user %s not met vote gate requirements", user.Wallet)
		log.Err(err)
		sdk.LogForbiddenError(ctx, user.Wallet, fmt.Sprintf("vote in proposal %s", proposalIdString), "cast_vote")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	reqData := CastVoteData{}
	if err := ctx.BindJSON(&reqData); err != nil {
		log.Error().Msgf("parse request data error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
		return
	}

	if err := metaforo.CastVote(
		reqData.MetaforoAccessToken,
		cfg.MetaforoData.GroupName,
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
	_, cfg := api.ForContextDBAndConfig(ctx)
	reqData := RevokeVoteData{}
	if err := ctx.BindJSON(&reqData); err != nil {
		log.Error().Msgf("parse request data error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
		return
	}

	if err := metaforo.RevokeVote(
		reqData.MetaforoAccessToken,
		cfg.MetaforoData.GroupName,
		reqData.MetaforoVoteId,
	); err != nil {
		log.Error().Msgf("revoke vote error error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("revoke vote error")))
		return

	}
	ctx.JSON(http.StatusOK, api.Success(nil))
}

// CloseVote closes all the votes in the proposal
//
//	@summary	close all votes belongs to the proposal
//	@tags		Proposal
//	@param		id		query		number				true	"proposal ID"
//	@param		data	body		CloseVoteRequest	true	"revoke vote data"
//	@success	200		{object}	api.Reply{data=nil}	"Success"
//	@router		/proposals/close_vote/:id [post]
func CloseVote(ctx *gin.Context) {
	db, cfg := api.ForContextDBAndConfig(ctx)
	proposalIdStr := ctx.Param("id")
	reqData := CloseVoteRequest{}
	if err := ctx.BindJSON(&reqData); err != nil {
		log.Error().Msgf("parse request data error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
		return
	}

	if err := metaforo.CloseVote(
		cfg.MetaforoData.AccessToken,
		cfg.MetaforoData.GroupName,
		reqData.MetaforoVoteId,
	); err != nil {
		log.Error().Msgf("close vote error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("close vote error")))
		return
	}

	// update proposal state after getting the vote result
	dbProposal, err := GetProposalFromStringId(db, proposalIdStr)
	if err != nil {
		log.Error().Msgf("fetch db proposal record error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("fetch db proposal error")))
		return
	}

	metaforoProposalResponse, err := metaforo.GetProposal(dbProposal.GetMetaforoThreadId(), cfg.MetaforoData.GroupName, "", 0)
	err = UpdateDbRecordsFromMetaforoProposalResponse(db, dbProposal, metaforoProposalResponse)
	if err != nil {
		log.Error().Msgf("update db proposal by response error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("update proposal info error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// ShowVoteDetail returns vote detail for specified vote
//
//	@summary	revoke vote on existing metaforo vote
//	@tags		Proposal
//	@param		vote_option_id	path		number										true	"Vote ID"
//	@param		page			query		number										false	"page of the vote list"
//	@success	200				{object}	api.Reply{data=[]JointMetaforoAndOsUser}	"Success"
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

	db, cfg := api.ForContextDBAndConfig(ctx)

	voterList, err := metaforo.GetVoterList(cfg.MetaforoData.GroupName, voteId, page)
	if err != nil {
		log.Error().Msgf("get vote list error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("get vote list error")))
		return
	}

	metaforoUserIds := lo.Map(voterList, func(item *metaforo.UserPollRecord, index int) int { return item.UserId })
	userRecords, err := GetOsUserFromMetaforoUserId(db, metaforoUserIds)
	ctx.JSON(http.StatusOK, api.Success(userRecords))
}

func canUserVoteOnThread(db *gorm.DB, userWallet string, proposalIdString string) (bool, error) {
	proposal, err := GetProposalFromStringId(db, proposalIdString)
	if err != nil {
		log.Error().Msgf("get proposal error: %+v", err)
		return false, err
	}

	// Verify NFT gate
	seepassData, err := api.GetCachedSeepassData(sdk.GetSppClient(), userWallet, false)
	if err != nil {
		log.Error().Msgf("get seepass data error: %+v", err)
		return false, err
	}

	var proposalCategory *model.ProposalCategory
	err = db.Model(&model.ProposalCategory{}).
		Joins("ProposalVoteGate").
		Where(model.ProposalCategory{ID: proposal.ProposalCategoryID}).First(&proposalCategory).Error

	return IsUserMetVoteGate(seepassData, proposalCategory.ProposalVoteGate), nil
}
