package sns_invite

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/service"
)

type GetMySnsInviteCodeReply struct {
	InviteCode string `json:"invite_code"`
}

// GetMySnsInviteCode get my sns invite code
//
//	@Summary		get my sns invite code
//	@Description	get my sns invite code
//	@Tags			SnsInvite
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	api.Reply{data=GetMySnsInviteCodeReply}
//	@Router			/sns_invite/my_sns_invite_code [get]
func GetMySnsInviteCode(ctx *gin.Context) {
	user, db := api.ForContextUserAndDB(ctx)

	inviteCode, err := service.GetMySnsInviteCode(db, user.Wallet)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get my invite code error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&GetMySnsInviteCodeReply{inviteCode}))
}

type GetMySnsInviteRewardsReply struct {
	InviteCount  int    `json:"invite_count"`
	TotalRewards string `json:"total_rewards"`
}

// GetMySnsInviteRewards get my sns invite rewards
//
//	@Summary		get my sns invite rewards
//	@Description	get my sns invite rewards
//	@Tags			SnsInvite
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	api.Reply{data=GetMySnsInviteRewardsReply}
//	@Router			/sns_invite/my_sns_invite_rewards [get]
func GetMySnsInviteRewards(ctx *gin.Context) {
	user, db := api.ForContextUserAndDB(ctx)

	inviteCount, totalRewards, err := service.GetMySnsInviteRewards(db, user.Wallet)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get my invite rewards error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&GetMySnsInviteRewardsReply{inviteCount, totalRewards.String()}))
}

// SnsInvitedBy
//
//	@Summary		invited by someone for register sns
//	@Description	invited by someone for register sns
//	@Tags			SnsInvite
//	@Accept			json
//	@Produce		json
//	@Param			invite_code	path		string	true	"invite code"
//	@Success		200			{object}	api.Reply
//	@Router			/sns_invite/invited_by/{invite_code} [post]
func SnsInvitedBy(ctx *gin.Context) {
	user, db := api.ForContextUserAndDB(ctx)
	if !model.IsSnsInvitationEnabled(db) {
		ctx.JSON(http.StatusOK, api.Success(nil))
		return
	}

	inviteCode := ctx.Param("invite_code")

	err := service.SnsInvitedBy(db, inviteCode, user.Wallet)
	if errors.Is(err, service.ErrInvalidInviteCode) || errors.Is(err, service.ErrAlreadyInvited) {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("sns invited by error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}
