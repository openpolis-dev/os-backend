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

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Auth ------ ------

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

// Login `POST /login`
func Login(ctx *gin.Context) {
	req := LoginReq{}
	_ = ctx.BindJSON(&req)

	// verify sign
	err := common.VerifyWalletSign(req.Wallet, req.Timestamp, req.Sign)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, err)
		return
	}

	_, db, cfg := api.ForContext(ctx)

	// query user
	user, err := model.UserModel.Detail(db, req.Wallet)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}
	if user == nil {
		user = &model.User{
			Wallet: strings.ToLower(req.Wallet),
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

	ctx.JSON(http.StatusOK, api.Success(LoginReply{
		Token:    token,
		TokenExp: tokenExp,
		User:     user,
	}))
}

// Logout `GET /logout`
func Logout(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, api.Success(nil))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ User ------ ------

// Detail `GET /me`
func Detail(ctx *gin.Context) {
	user, db, _ := api.ForContext(ctx)

	u, err := model.UserModel.Detail(db, user.Wallet)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, api.Success(u))
}

type UpdateReq struct {
	Name           string `json:"name"`
	Avatar         string `json:"avatar"`
	Email          string `json:"email"`
	DiscordProfile string `json:"discordProfile"`
	TwitterProfile string `json:"twitterProfile"`
	GoogleProfile  string `json:"GoogleProfile"`
}

// Update `PUT /me`
func Update(ctx *gin.Context) {
	req := UpdateReq{}
	_ = ctx.BindJSON(&req)

	user, db, _ := api.ForContext(ctx)

	u, err := model.UserModel.Detail(db, user.Wallet)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}
	// update user info
	u.Name = req.Name
	u.Avatar = req.Avatar
	u.Email = req.Email
	u.DiscordProfile = req.DiscordProfile
	u.TwitterProfile = req.TwitterProfile
	u.GoogleProfile = req.GoogleProfile
	err = model.UserModel.CreateOrUpdate(db, u)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// Users `GET /users?wallets=0x1&wallets=0x2&wallets=0x3`
// query multiple users by wallet array on batch
func Users(ctx *gin.Context) {
	wallets := ctx.QueryArray("wallets")

	db := api.ForContextOnlyDB(ctx)

	users, err := model.UserModel.List(db, wallets)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, api.Success(users))
}
