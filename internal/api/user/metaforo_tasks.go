package user

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
)

// MetaforoActivities returns metaforo activities by given user id
//
//	@summary	Get metaforo activities
//	@router		/user/metaforo_activities [get]
//	@tags		metaforo
//	@params		userId query string true "Metaforo user id"
//	@params		size query int true "Size of activities"
//	@params		session query string false "params for next page"
//	@success	200	{object}	api.Reply{data=metaforo.UserActivity}
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

// UpdateMetaforoData received metaforo login response data sent from frontend side and build associated relationship between metaforo
//
//	@summary	build metaforo account relationship between OS user
//	@router		/user/update_metaforo_data [post]
//	@tags		user
//	@tags		metaforo
//	@Param		JsonBody	body		metaforo.User	true	"user data from login request"
//	@success	200			{object}	api.Reply{data=nil}
func UpdateMetaforoData(ctx *gin.Context) {
	metaforoUser := metaforo.User{}
	err := ctx.BindJSON(&metaforoUser)
	if err != nil {
		log.Error().Msgf("parse metaforo user error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	userGroupsBytes, err := json.Marshal(metaforoUser.GroupProfiles)
	if err != nil {
		log.Error().Msgf("parse metaforo user error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("parse user info error")))
		return
	}

	user, _, db, _ := api.ForContext(ctx)
	err = db.Model(model.MetaforoUser{}).
		Where(&model.MetaforoUser{UserWallet: common.FormatUserWallet(user.Wallet)}).
		Updates(&model.MetaforoUser{
			MetaforoUserId: metaforoUser.Id,
			Groups:         userGroupsBytes,
		}).
		Error

	if err != nil {
		log.Error().Msgf("update metaforo user error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update user info error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}
