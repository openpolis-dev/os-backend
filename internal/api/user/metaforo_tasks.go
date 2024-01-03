package user

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/api/proposal"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
)

type JoinOrLeaveGroupReq struct {
	GroupName           string `json:"group_name"`
	MetaforoAccessToken string `json:"metaforo_access_token"`
}

type MetaforoActivityRecord struct {
	Wallet           string `json:"wallet"`
	ProposalID       uint   `json:"proposal_id"`
	Action           string `json:"metaforo_action"`
	ThreadTitle      string `json:"target_title"`
	MetaforoThreadId int    `json:"metaforo_thread_id"`
	ActionTs         int64  `json:"action_ts"`
}

type MetaforoActivityResponse struct {
	Session string                    `json:"session"`
	Records []*MetaforoActivityRecord `json:"records"`
}

// MetaforoActivities returns metaforo activities by given user id
//
//	@summary	Get metaforo activities
//	@router		/user/metaforo_activities [get]
//	@tags		Metaforo
//	@param		userId	query		string	true	"Metaforo user id"
//	@param		size	query		int		true	"Size of activities"
//	@param		session	query		string	false	"params for next page"
//	@success	200		{object}	api.Reply{data=MetaforoActivityRecord}
func MetaforoActivities(ctx *gin.Context) {
	var err error

	userId := ctx.Query("userId")
	size := ctx.Query("size")
	session := ctx.Query("session")

	if userId == "" {
		log.Error().Msgf("missing metaforo user id")
		sdk.LogUserSideError(ctx, errors.New("missing metaforo user id"))
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("missing metaforo user id")))
		return
	}

	userIdVal, err := strconv.Atoi(userId)
	if err != nil {
		log.Error().Msgf("parse user id error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("parse user id error")))
		return
	}

	sizeVal := 0
	if size != "" {
		sizeVal, err = strconv.Atoi(size)
		if err != nil {
			log.Error().Msgf("parse size error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("parse size error")))
			return
		}
	}

	if sizeVal == 0 {
		sizeVal = internal.DefaultPageSize
	}

	activities, newSession, err := metaforo.UserActivities(userId, "all", fmt.Sprintf("%d", sizeVal), session)
	if err != nil {
		log.Error().Msgf("get metaforo activities error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get activities error")))
		return
	}

	// Save activity records to
	userIdActivityMap := make(map[int]*metaforo.UserActivity)
	var metaforoUserIds []int
	for _, activity := range activities {
		userIdActivityMap[activity.UserId] = activity
		metaforoUserIds = append(metaforoUserIds, activity.UserId)
	}

	db := api.ForContextOnlyDB(ctx)
	// Get proposalIds from metaforo thread id in activities
	proposalRecordIds := lo.Map(activities, func(r *metaforo.UserActivity, _ int) string {
		return model.BuildProposalRecordIdFromMetaforoThreadId(r.ThreadId)
	})

	var touchedProposals []*model.Proposal
	err = db.Model(model.Proposal{}).Where("proposal_record_id in ?", proposalRecordIds).Select("id, proposal_record_id").Find(&touchedProposals).Error
	if err != nil {
		log.Error().Msgf("get proposal error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal error")))
		return
	}

	metaforoThreadIdToProposalIdMapping := make(map[int]uint)
	for _, proposal := range touchedProposals {
		metaforoThreadIdToProposalIdMapping[proposal.GetMetaforoThreadId()] = proposal.ID
	}

	userRecords, err := proposal.GetOsUserFromMetaforoUserId(db, []int{userIdVal})
	if err != nil {
		log.Error().Msgf("get os user error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get os user error")))
		return
	}

	wallet := ""
	if len(userRecords) == 0 {
		log.Warn().Msgf("metaforo user id %d not found in DB", userIdVal)
	} else {
		wallet = userRecords[0].Wallet
	}

	rsltRcds := make([]*MetaforoActivityRecord, 0)
	for idx := range activities {
		r := activities[idx]
		if r.GroupId != internal.MetaforoGroupId {
			continue
		}

		action := "unknown"
		if r.LikeId != 0 {
			action = "like"
		} else if r.FirstPostId == r.Id {
			action = "create"
		} else {
			action = "comment"
		}

		rsltRcds = append(rsltRcds, &MetaforoActivityRecord{
			Wallet:           wallet,
			ProposalID:       metaforoThreadIdToProposalIdMapping[r.ThreadId],
			Action:           action,
			ThreadTitle:      r.ThreadTitle,
			MetaforoThreadId: r.ThreadId,
			ActionTs:         r.CreatedAt.UTC().Unix(),
		})
	}

	ctx.JSON(http.StatusOK, api.Success(&MetaforoActivityResponse{
		Session: newSession,
		Records: rsltRcds,
	}))
}

// JoinMetaforoGroup allow user joins metaforo group
//
//	@router		/user/join_metaforo_group [post]
//	@summary	Join a Metaforo group
//	@tags		Metaforo
//	@param		req	body		JoinOrLeaveGroupReq	true	"Join group request body"
//	@success	200	{object}	api.Reply{data=nil}
func JoinMetaforoGroup(ctx *gin.Context) {
	var req JoinOrLeaveGroupReq
	err := ctx.BindJSON(&req)
	if err != nil {
		sdk.LogUserSideError(ctx, err)
		log.Error().Msgf("parse join group request error: %+v", err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	err = metaforo.JoinGroup(req.MetaforoAccessToken, req.GroupName)
	if err != nil {
		log.Error().Msgf("join group error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("join group error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// LeaveMetaforoGroup allow user joins metaforo group
//
//	@router		/user/leave_metaforo_group [post]
//	@summary	Leave a Metaforo group
//	@tags		Metaforo
//	@param		req	body		JoinOrLeaveGroupReq	true	"Join group request body"
//	@success	200	{object}	api.Reply{data=nil}
func LeaveMetaforoGroup(ctx *gin.Context) {
	var req JoinOrLeaveGroupReq
	err := ctx.BindJSON(&req)
	if err != nil {
		sdk.LogUserSideError(ctx, err)
		log.Error().Msgf("parse leave group request error: %+v", err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	err = metaforo.LeaveGroup(req.MetaforoAccessToken, req.GroupName)
	if err != nil {
		log.Error().Msgf("leave group error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("leave group error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// PrepareMetaforoData accepts user login data, saves data mapping between metaforo user ID and wallet, then invoke join group API
//
//	@summary	Upload metaforo user data to link with os user, and invoke join group
//	@router		/user/prepare_metaforo [post]
//	@tags		User
//	@tags		Metaforo
//	@Param		JsonBody	body		metaforo.LoginResponse	true	"metaforo response after login"
//	@success	200			{object}	api.Reply{data=nil}
func PrepareMetaforoData(ctx *gin.Context) {
	var req metaforo.LoginResponse
	err := ctx.BindJSON(&req)
	if err != nil {
		sdk.LogUserSideError(ctx, err)
		log.Error().Msgf("parse prepare metaforo user request error: %+v", err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	userGroupsBytes, err := json.Marshal(req.User.GroupProfiles)
	if err != nil {
		log.Error().Msgf("serialize metaforo user group info error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("processing user info error")))
		return
	}

	user, _, db, _ := api.ForContext(ctx)
	if strings.EqualFold(user.Wallet, req.User.Web3PublicKey) {
		err := fmt.Errorf("login user %s do not equals to metaforo user %s", user.Wallet, req.User.Web3PublicKey)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	var metaforoUser model.MetaforoUser
	err = db.Model(model.MetaforoUser{}).Where(model.MetaforoUser{
		MetaforoUserId: req.User.Id,
		UserWallet:     common.FormatUserWallet(req.User.Web3PublicKey),
	}).Attrs(model.MetaforoUser{
		Groups: userGroupsBytes,
	}).FirstOrCreate(&metaforoUser).Error

	if err != nil {
		log.Error().Msgf("update metaforo user error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update user info error")))
		return
	}

	err = metaforo.JoinGroup(req.ApiToken, internal.MetaforoGroupName)
	if err != nil {
		log.Error().Msgf("join group error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("join group error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}
