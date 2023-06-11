package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
)

type LoginReq struct {
	Wallet    string `json:"wallet" binding:"required"`
	Timestamp int64  `json:"timestamp" binding:"required"` // time unit: seconds
	Sign      string `json:"sign" binding:"required"`
}

type LoginReply struct {
	Token    string      `json:"token"`
	TokenExp int64       `json:"tokenExp"` // time unit: seconds
	User     *model.User `json:"user"`
}

func Login(ctx *gin.Context) {
	req := LoginReq{}
	_ = ctx.BindJSON(&req)

	// verify sign
	err := common.VerifyWalletSign(req.Wallet, req.Timestamp, req.Sign)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, err)
		return
	}

	//wallet, db, cfg := ForContext(ctx)
	_, db, cfg := ForContext(ctx)

	// query user
	user, err := model.UserModel.User(db, req.Wallet)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}
	if user == nil {
		user = &model.User{
			Wallet:   strings.ToLower(req.Wallet),
			Username: "",
			Email:    "",
			Avatar:   "",
		}
		err = model.UserModel.CreateOrUpdate(db, user)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, err)
			return
		}
	}

	// generate jwt token
	token, tokenExp, err := common.GenerateJwtToken[middleware.CurUser](
		&middleware.CurUser{
			Wallet: req.Wallet,
		},
		time.Duration(cfg.Jwt.Exp)*time.Hour,
		cfg.Jwt.Secret,
	)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, err)
		return
	}

	ctx.JSON(http.StatusOK, Success(LoginReply{
		Token:    token,
		TokenExp: tokenExp,
		User:     user,
	}))
}

func Logout(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, Success(nil))
}
