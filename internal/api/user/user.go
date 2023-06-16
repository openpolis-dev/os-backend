package user

import (
	"bytes"
	"encoding/json"
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
	TokenExp int64       `json:"token_exp"` // time unit: seconds
	User     *model.User `json:"user"`
}

// Login `POST /login`
func Login(ctx *gin.Context) {
	req := LoginReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	// verify sign
	err = common.VerifyWalletSign(req.Wallet, req.Timestamp, req.Sign)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	_, _, db, cfg := api.ForContext(ctx)

	// query user
	user, err := model.UserModel.Detail(db, strings.ToLower(req.Wallet))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if user == nil {
		user = &model.User{
			Wallet: strings.ToLower(req.Wallet),
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

// Logout `GET /logout`
func Logout(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, api.Success(nil))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ User ------ ------

// Detail `GET /me`
func Detail(ctx *gin.Context) {
	user, db := api.ForContextUserAndDB(ctx)

	u, err := model.UserModel.Detail(db, user.Wallet)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(u))
}

type UpdateReq struct {
	Name           string `json:"name"`
	Avatar         string `json:"avatar"`
	Email          string `json:"email"`
	DiscordProfile string `json:"discord_profile"`
	TwitterProfile string `json:"twitter_profile"`
	GoogleProfile  string `json:"google_profile"`
}

// Update `PUT /me`
func Update(ctx *gin.Context) {
	req := UpdateReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	user, db := api.ForContextUserAndDB(ctx)

	u, err := model.UserModel.Detail(db, user.Wallet)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
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
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
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
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(users))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Permission ------ ------

// GetFrontendPermission `GET /casbin?casbin_subject=0x1`
// query frontend permission by user wallet
func GetFrontendPermission(ctx *gin.Context) {
	_, enforcer, _, _ := api.ForContext(ctx)

	//sub, _ := ctx.GetQuery("casbin_subject")
	//data, err := casbin.CasbinJsGetPermissionForUser(enforcer, strings.ToLower(sub))
	// --> casbin.CasbinJsGetPermissionForUser()'s logic is not correct, should use the following logic instead:
	eModel := enforcer.GetModel()
	m := map[string]interface{}{}
	m["m"] = eModel.ToText()
	policies := make([][]string, 0)
	for ptype := range eModel["p"] {
		policy := eModel.GetPolicy("p", ptype)
		for i := range policy {
			policies = append(policies, append([]string{ptype}, policy[i]...))
		}
	}
	for ptype := range eModel["g"] {
		role := eModel.GetPolicy("g", ptype)
		for i := range role {
			policies = append(policies, append([]string{ptype}, role[i]...))
		}
	}
	m["p"] = policies
	result := bytes.NewBuffer([]byte{})
	encoder := json.NewEncoder(result)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(m)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(result.String()))
}
