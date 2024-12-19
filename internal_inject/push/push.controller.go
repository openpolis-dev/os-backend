package push_inject

import (
	"errors"
	"net/http"
	"time"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type PushController struct {
	// inject
	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	PushService *PushService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var push PushController

	err := inject.Populate(&push, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var pushAuthGroup *gin.RouterGroup

	if fatherGroup != nil {
		pushAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/push")
	} else {
		pushAuthGroup = push.Gin.Group("/", middleware.AuthRequired).Group("/push")
	}

	// auth
	pushAuthGroup.POST("/", push.Create)
	pushAuthGroup.GET("/", push.List)
}

func (c *PushController) Create(ctx *gin.Context) {
	req := CreateReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	user, enforcer, _, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), internal.ObjPush, internal.ActCreatePush)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.ObjPush, internal.ActCreatePush)
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	// save push
	push := model.Push{
		CreatorWallet: user.Wallet,
		Title:         req.Title,
		Content:       req.Content,
		JumpURL:       req.JumpURL,
		PushDate:      time.Now(),
		//Status: 0,
		CreatedAt: time.Now().In(internal.ProjectTimezone),
		CreateTs:  model.GetCurrentUtcEpochSecond(),
		UpdatedAt: time.Now().In(internal.ProjectTimezone),
		UpdateTs:  model.GetCurrentUtcEpochSecond(),
	}
	err = model.PushModel.CreateOrUpdate(c.Db, &push)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create push error")))
		return
	}

	// send push
	pushSDK := api.ForContextOnlyPush(ctx)
	go func(pushSDK []sdk.Pusher, title, body map[string]string, jumpURL string) {
		data := sdk.GenerateCustomNotificationParams(jumpURL)
		for _, p := range pushSDK {
			err := p.PushAll(title, body, data)
			if err != nil {
				log.Error().Msgf("push to all failed: %s", err)
			}
		}
	}(pushSDK, map[string]string{sdk.LanguageZH: req.Title, sdk.LanguageEN: req.Title}, map[string]string{sdk.LanguageZH: req.Content, sdk.LanguageEN: req.Content}, req.JumpURL)

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func (c *PushController) List(ctx *gin.Context) {
	page := api.ParseAndConvertPageParam(ctx)

	pushes, total, err := model.PushModel.List(c.Db, nil, page)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list push error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&api.ListReplyData{
		Page:  page.Page,
		Size:  page.Size,
		Total: total,
		Rows:  pushes,
	}))
}
