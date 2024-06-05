package proposal

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type VoterInfo struct {
	MetaforoUserId int    `json:"metaforo_user_id"`
	Wallet         string `json:"wallet"`
	Avatar         string `json:"avatar"`
}

type userVoteDetailInfo struct {
	JointMetaforoAndOsUser
	VoteWeight int `json:"weight"`
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
		reqData.MetaforoAccessToken,
		cfg.MetaforoData.GroupName,
		reqData.MetaforoVoteId,
	); err != nil {
		log.Error().Msgf("close vote error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("close vote error")))
		return
	}

	// update proposal state after getting the vote result
	dbProposal, metaforoProposalResponse, err := GetMetaforoProposalByInternalId(db, proposalIdStr, cfg.MetaforoData.GroupName, reqData.MetaforoAccessToken)
	pollStatusChanged, err := UpdateDbVoteOptionRecordsFromMetaforoProposalResponse(db, dbProposal.ID, metaforoProposalResponse)
	if err != nil {
		log.Error().Msgf("update propsal vote option records with metaforo response error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("close vote error")))
		return
	}

	if pollStatusChanged {
		if err = HandleProposalPollStatusChange(db, dbProposal.ID); err != nil {
			log.Error().Msgf("handle proposal poll status change error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("close vote error")))
			return
		}
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// ShowVoteDetail returns vote detail for specified vote
//
//	@summary	show voter detail for specified vote option
//	@tags		Proposal
//	@param		vote_option_id	path		number										true	"Vote ID"
//	@param		page			query		number										false	"page of the vote list"
//	@success	200				{object}	api.Reply{data=[]userVoteDetailInfo}	"Success"
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

	// Update metaforo user record if UID not found in DB
	missingMfUserIds := map[int]*metaforo.UserDetailResponseForProfileAPI{}
	for _, mfVoterRecord := range voterList {
		var rcdCount int64
		if err = db.Model(&model.MetaforoUser{}).Where(&model.MetaforoUser{MetaforoUserId: mfVoterRecord.UserId}).Count(&rcdCount).Error; err != nil {
			log.Error().Msgf("count metaforo user record error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("get metaforo user record error")))
			return
		}
		if rcdCount == 0 {
			mfUserData, err := metaforo.UserDetail(mfVoterRecord.UserId)
			if err != nil {
				log.Error().Msgf("get metaforo user detail error: %+v", err)
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("get metaforo user detail error")))
				return
			}
			missingMfUserIds[mfVoterRecord.UserId] = mfUserData
		}
	}

	// Create MetaforoUser and User record from API data
	err = db.Transaction(func(tx *gorm.DB) error {
		for userId, profileData := range missingMfUserIds {
			metaforoUser := model.MetaforoUser{
				MetaforoUserId: userId,
				UserWallet:     common.FormatUserWallet(profileData.User.Web3PublicKey),
			}
			mfUserTx := tx.Where(&model.MetaforoUser{MetaforoUserId: userId}).Find(&metaforoUser)

			if mfUserTx.Error != nil {
				log.Error().Msgf("get metaforo user record error: %+v", mfUserTx.Error)
				sdk.LogServerErrorToSentry(ctx, mfUserTx.Error)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("get metaforo user record error")))
				return mfUserTx.Error
			} else if mfUserTx.RowsAffected == 0 {
				if err = tx.Create(&metaforoUser).Error; err != nil {
					log.Error().Msgf("create metaforo user record error: %+v", err)
					sdk.LogServerErrorToSentry(ctx, err)
					ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("create metaforo user record error")))
					return err
				}
			} else if mfUserTx.RowsAffected == 1 {
				if err = tx.Updates(&metaforoUser).Error; err != nil {
					log.Error().Msgf("update metaforo user record error: %+v", err)
					sdk.LogServerErrorToSentry(ctx, err)
					ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("update metaforo user record error")))
					return err
				}
			} else {
				err = fmt.Errorf("unexpected rows affected: %d", mfUserTx.RowsAffected)
				log.Error().Msgf(err.Error())
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("unexpected rows affected: %d", mfUserTx.RowsAffected)))
				return err
			}

			// Create user record if not existing
			userRecord := model.User{Wallet: metaforoUser.UserWallet}
			db.Clauses(clause.OnConflict{DoNothing: true}).Model(&model.User{}).Where(&userRecord).Assign(model.User{
				CreateTs: model.GetCurrentUtcEpochSecond(),
				UpdateTs: model.GetCurrentUtcEpochSecond(),
				Avatar:   profileData.User.PhotoUrl,
				Name:     profileData.User.Username,
			}).FirstOrCreate(&userRecord)
		}
		return nil
	})

	// Extract weight value for each user
	userWeightMap := lo.SliceToMap(voterList, func(item *metaforo.UserPollRecord) (int, int) { return item.Uid, item.Weight })

	metaforoUserIds := lo.Map(voterList, func(item *metaforo.UserPollRecord, index int) int { return item.UserId })
	userRecords, err := GetOsUserFromMetaforoUserId(db, metaforoUserIds)
	if err != nil {
		log.Error().Msgf("list user vote detail error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("list user vote detail error")))
		return
	}

	rslt := lo.Map(userRecords, func(u *JointMetaforoAndOsUser, _ int) *userVoteDetailInfo {
		weight, found := userWeightMap[u.MetaforoUserID]
		if !found {
			log.Warn().Msgf("no weight found for user: %d", u.MetaforoUserID)
			weight = 0
		}
		return &userVoteDetailInfo{
			*u,
			weight,
		}
	})
	ctx.JSON(http.StatusOK, api.Success(rslt))
}

func canUserVoteOnThread(db *gorm.DB, userWallet string, proposalIdString string) (bool, error) {
	proposal, err := GetProposalFromStringId(db, proposalIdString)
	if err != nil {
		log.Error().Msgf("get proposal error: %+v", err)
		return false, err
	}
	log.Debug().Msgf("check voting permission of %s for proposal: %+v", userWallet, proposal)

	// Verify NFT gate
	seepassData, err := api.GetCachedSeepassData(sdk.GetSppClient(), userWallet, false)
	if err != nil {
		log.Error().Msgf("get seepass data error: %+v", err)
		return false, err
	}

	var voteGates []*model.ProposalVoteGate
	pTmplDbRcd := model.ProposalTemplate{
		ID: *proposal.ProposalTemplateID,
	}

	err = db.Model(&pTmplDbRcd).Association("VoteGates").Find(&voteGates)
	if err != nil {
		log.Error().Msgf("get proposal template error: %+v", err)
		return false, err
	}

	permArray := lo.Map(voteGates, func(r *model.ProposalVoteGate, _ int) bool {
		return IsUserMetVoteGate(seepassData, r)
	})
	log.Debug().Msgf("voting perm array of %s for proposal: %d is %+v", userWallet, proposal.ID, permArray)

	permResult := lo.Reduce(permArray, func(rslt bool, r bool, _ int) bool {
		return rslt && r
	}, true)
	log.Debug().Msgf("voting perm result of %s for proposal: %d is %+v", userWallet, proposal.ID, permResult)

	return permResult, nil
}
