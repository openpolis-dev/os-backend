package user

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
)

type JoinOrLeaveGroupReq struct {
	GroupName           string `json:"group_name"`
	MetaforoAccessToken string `json:"metaforo_access_token"`
}

// MetaforoActivities returns metaforo activities by given user id
//
//	@summary	Get metaforo activities
//	@router		/user/metaforo_activities [get]
//	@tags		Metaforo
//	@param		userId	query		string	true	"Metaforo user id"
//	@param		size	query		int		true	"Size of activities"
//	@param		session	query		string	false	"params for next page"
//	@success	200		{object}	api.Reply{data=metaforo.UserActivity}
func MetaforoActivities(ctx *gin.Context) {
	userId := ctx.Param("userId")
	size := ctx.Param("size")
	session := ctx.Param("session")
	activities, err := metaforo.UserActivities(userId, "all", size, session)
	if err != nil {
		log.Error().Msgf("get metaforo activities error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get activities error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(activities))
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

	var metaforoUser metaforo.User
	err = db.Where(model.MetaforoUser{
		MetaforoUserId: req.User.Id,
		UserWallet:     common.FormatUserWallet(req.User.Web3PublicKey),
	}).Attrs(model.MetaforoUser{
		Groups: userGroupsBytes,
	}).FirstOrInit(&metaforoUser).Error

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
