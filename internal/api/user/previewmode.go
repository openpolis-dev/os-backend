package user

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
)

// preview mode flag
// - default value is `false`
var previewEnable = false

type PreviewEnableReply struct {
	Enable bool `json:"enable"`
}

// PreviewEnable `GET /preview_enable`
func PreviewEnable(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, api.Success(PreviewEnableReply{Enable: previewEnable}))
}

type PreviewToggleReply struct {
	OldValue bool `json:"oldValue"`
	NewValue bool `json:"newValue"`
}

// PreviewToggle `PUT /preview_toggle`
func PreviewToggle(ctx *gin.Context) {
	oldValue := previewEnable
	previewEnable = !previewEnable

	ctx.JSON(http.StatusOK, api.Success(PreviewToggleReply{OldValue: oldValue, NewValue: previewEnable}))
}

// PreviewLogin `POST /preview_login`
func PreviewLogin(ctx *gin.Context) {
	_, _, db, cfg := api.ForContext(ctx)

	// preview wallet
	previewWallet := strings.ToLower(cfg.PreviewMode.Wallet)

	// query user
	user, err := model.UserModel.Detail(db, previewWallet)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if user == nil {
		user = &model.User{
			Wallet: previewWallet,
		}
		err = model.UserModel.CreateOrUpdate(db, user)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
	}

	// generate jwt token
	token, tokenExp, err := common.GenerateJwtToken[middleware.CurUser](
		&middleware.CurUser{
			Wallet: user.Wallet,
		},
		time.Duration(cfg.Jwt.Exp)*time.Hour,
		cfg.Jwt.Secret,
	)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(LoginReply{
		Token:    token,
		TokenExp: tokenExp,
		User:     user,
	}))
}
