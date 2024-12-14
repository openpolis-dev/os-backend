package snsinvite_inject

import (
	"errors"
	"net/http"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/service"
	"gorm.io/gorm"
)

type SnsInviteController struct {
	// inject
	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	SnsInviteService *SnsInviteService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var snsInvite SnsInviteController

	err := inject.Populate(&snsInvite, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var snsInviteAuthGroup *gin.RouterGroup

	if fatherGroup != nil {
		snsInviteAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/sns_invite")
	} else {
		snsInviteAuthGroup = snsInvite.Gin.Group("/", middleware.AuthRequired).Group("/sns_invite")
	}

	// auth
	snsInviteAuthGroup.GET("/my_sns_invite_code", snsInvite.GetMySnsInviteCode)
	snsInviteAuthGroup.GET("/my_sns_invite_rewards", snsInvite.GetMySnsInviteRewards)
	snsInviteAuthGroup.POST("/invited_by/:invite_code", snsInvite.SnsInvitedBy)
}

func (c *SnsInviteController) GetMySnsInviteCode(ctx *gin.Context) {
	user, _ := api.ForContextUserAndDB(ctx)

	inviteCode, err := service.GetMySnsInviteCode(c.Db, user.Wallet)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get my invite code error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&GetMySnsInviteCodeReply{inviteCode}))
}

func (c *SnsInviteController) GetMySnsInviteRewards(ctx *gin.Context) {
	user, _ := api.ForContextUserAndDB(ctx)

	inviteCount, totalRewards, err := service.GetMySnsInviteRewards(c.Db, user.Wallet)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get my invite rewards error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&GetMySnsInviteRewardsReply{inviteCount, totalRewards.String()}))

}

func (c *SnsInviteController) SnsInvitedBy(ctx *gin.Context) {
	user, _ := api.ForContextUserAndDB(ctx)
	if !model.IsSnsInvitationEnabled(c.Db) {
		ctx.JSON(http.StatusOK, api.Success(nil))
		return
	}

	inviteCode := ctx.Param("invite_code")

	err := service.SnsInvitedBy(c.Db, inviteCode, user.Wallet)
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
