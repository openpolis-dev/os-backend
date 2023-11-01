package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	eth_common "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/spruceid/siwe-go"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	unipass_sigverify "github.com/unipassid/unipass-sigverify-go"
)

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Sign in with Ethereum ------ ------

type RefreshNonceReq struct {
	Wallet string `json:"wallet"`
}

type RefreshNonceReply struct {
	Nonce string `json:"nonce"`
}

// RefreshNonce refresh nonce
// @Summary Refresh nonce
// @Tags Auth
// @Accept json
// @Produce json
// @Param JsonBody body RefreshNonceReq true "request json body"
// @Success 200 {object} Reply
// @Router /v1/refresh_nonce [post]
func RefreshNonce(ctx *gin.Context) {
	req := RefreshNonceReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	db := api.ForContextOnlyDB(ctx)

	userNonce, err := model.UserNonceModel.Detail(db, req.Wallet)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	// new nonce and refreshAt
	nonce := siwe.GenerateNonce()
	refreshAt := time.Now().UnixMilli()
	// update with new value
	if userNonce == nil {
		userNonce = &model.UserNonce{Wallet: req.Wallet}
	}
	userNonce.Nonce = nonce
	userNonce.RefreshAt = refreshAt
	err = model.UserNonceModel.CreateOrUpdate(db, userNonce)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(RefreshNonceReply{Nonce: nonce}))
}

type RetrieveNonceReply struct {
	Nonce string `json:"nonce"`
}

// RetrieveNonce  retrieve nonce
//
//	`GET /retrieve_nonce?wallet=0x123
//
// @Summary Retrieve nonce
// @Tags Auth
// @Accept json
// @Produce json
// @Param wallet query string true "wallet address"
// @Success 200 {object} Reply
// @Router /v1/retrieve_nonce [get]
func RetrieveNonce(ctx *gin.Context) {
	wallet := ctx.Query("wallet")

	db, cfg := api.ForContextDBAndConfig(ctx)

	userNonce, err := model.UserNonceModel.RecentNonce(db, wallet, cfg.Auth.NonceLifespan)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	if userNonce == nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("no-login-request-recently")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(RetrieveNonceReply{Nonce: userNonce.Nonce}))
}

type LoginReq struct {
	Wallet         string `json:"wallet" binding:"required"`
	WalletType     string `json:"wallet_type" binding:"required"`
	IsEIP191Prefix bool   `json:"is_eip191_prefix"`
	Domain         string `json:"domain" binding:"required"`
	Message        string `json:"message" binding:"required"`
	Signature      string `json:"signature" binding:"required"`
}

type LoginReply struct {
	Token    string      `json:"token"`
	TokenExp int64       `json:"token_exp"` // time unit: seconds
	User     *model.User `json:"user"`
}

// Login user login
// @Summary Login
// @Tags Auth
// @Accept json
// @Produce json
// @Param JsonBody body LoginReq true "request json body"
// @Success 200 {object} Reply
// @Router /v1/login [post]
func Login(ctx *gin.Context) {
	req := LoginReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	_, _, db, cfg := api.ForContext(ctx)

	// verify sign
	// --> query nonce
	userNonce, err := model.UserNonceModel.RecentNonce(db, strings.ToLower(req.Wallet), cfg.Auth.NonceLifespan)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if userNonce == nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("please refresh nonce firstly")))
		return
	}
	if strings.EqualFold(req.WalletType, "EOA") {
		// --> verify signature
		message, err := siwe.ParseMessage(req.Message)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
			return
		}
		timestamp := time.Now()
		publicKey, err := message.Verify(req.Signature, &req.Domain, &userNonce.Nonce, &timestamp)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
			return
		}
		if !strings.EqualFold(req.Wallet, crypto.PubkeyToAddress(*publicKey).Hex()) {
			ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("signature not match")))
			return
		}
	} else if strings.EqualFold(req.WalletType, "AA") {
		client, err := ethclient.Dial(cfg.Auth.PolygonRPC)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}

		account := eth_common.HexToAddress(req.Wallet)
		sig := eth_common.FromHex(req.Signature)
		msg := []byte(req.Message)

		ok, err := unipass_sigverify.VerifyMessageSignature(context.Background(), account, msg, sig, req.IsEIP191Prefix, client)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
		if !ok {
			ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("signature not match")))
			return
		}
	}

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

// Logout user logout
// @Summary Logout
// @Tags Auth
// @Accept json
// @Produce json
// @Success 200 {object} Reply
// @Router /v1/logout [post]
func Logout(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, api.Success(nil))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ User ------ ------

// Detail get user detail
// @Summary Get user detail
// @Tags User
// @Accept json
// @Produce json
// @Success 200 {object} Reply
// @Router /v1/me [get]
func Detail(ctx *gin.Context) {
	// TODO: Get data from seepass API, and return data from DB if seepass returns 404
	user, db := api.ForContextUserAndDB(ctx)

	sppClient := sdk.GetSppClient()
	seepassResp, err := sppClient.GetSeepassData(user.Wallet)
	if err == nil {
		ctx.JSON(http.StatusOK, seepassResp)
		return
	}

	log.Warn().Msgf("query seepass data error, wallet: %s, error: %+v", user.Wallet, err)

	u, err := model.UserModel.Detail(db, user.Wallet)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if u == nil {
		u = &model.User{Wallet: user.Wallet}
	}

	seepassResp = &sdk.SeepassResponse{
		Roles:    make([]string, 0),
		Wallet:   user.Wallet,
		Nickname: u.Name,
		Avatar:   u.Avatar,
		Bio:      u.Bio,
		Email:    u.Email,
	}

	seepassResp.Scr.Amount = "0"

	// TODO: Move the hardcoded data to some const data or configuraiton service
	seepassResp.Level.CurrentLv = "0"
	seepassResp.Level.NextLv = "1"
	seepassResp.Level.ScrToNextLv = "5000"
	seepassResp.Level.UpgradePercent = "0"

	ctx.JSON(http.StatusOK, api.Success(seepassResp))
}

type UpdateReq struct {
	Name           string `json:"name"`
	Avatar         string `json:"avatar"`
	Bio            string `json:"bio"`
	Email          string `json:"email"`
	Wechat         string `json:"wechat"`
	DiscordProfile string `json:"discord_profile"`
	TwitterProfile string `json:"twitter_profile"`
	GoogleProfile  string `json:"google_profile"`
	Mirror         string `json:"mirror"`
}

// Update `PUT /me`
// @Summary Update user info
// @Tags User
// @Accept json
// @Produce json
// @Param JsonBody body UpdateReq true "request json body"
// @Success 200 {object} Reply
// @Router /v1/me [put]
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
	if u == nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("user %s not found", user.Wallet)))
		return
	}

	// update user info
	u.Name = req.Name
	u.Bio = req.Bio
	u.Email = req.Email
	u.Wechat = req.Wechat
	u.DiscordProfile = req.DiscordProfile
	u.TwitterProfile = req.TwitterProfile
	u.GoogleProfile = req.GoogleProfile
	u.Mirror = req.Mirror

	// Only upload image when data is b64 image string (start with `data:image`)
	avatarUrl, err := sdk.GetAwsClient().UploadUserAvatar(u.Wallet, req.Avatar)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	u.Avatar = avatarUrl

	err = model.UserModel.CreateOrUpdate(db, u)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	sppClient := sdk.GetSppClient()
	sppUpdatePayload := u.BuildSppUpdateProfilePayload()
	err = sppClient.UpdateProfile(user.Wallet, sppUpdatePayload)
	if err != nil {
		log.Error().Msgf("update user %s info to spp error, update req data: %+v, error: %+v", user.Wallet, req, err)
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// Users query multiple users by wallet array on batch
//
//	`GET /users?wallets=0x1&wallets=0x2&wallets=0x3`
//
// @Summary Query multiple users by wallet array on batch
// @Tags User
// @Accept json
// @Produce json
// @Param wallets query []string true "wallets"
// @Success 200 {object} Reply
// @Router /v1/users [get]
func Users(ctx *gin.Context) {
	wallets := ctx.QueryArray("wallets")
	//// convert all wallet to lower case
	//wallets := lo.Map[string](walletsParam, func(item string, _ int) string {
	//	return strings.ToLower(item)
	//})

	db := api.ForContextOnlyDB(ctx)

	users, err := model.UserModel.List(db, wallets)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	if len(users) != len(wallets) {
		existUsers := lo.Map[*model.User](users, func(item *model.User, _ int) string {
			return item.Wallet
		})
		for _, w := range wallets {
			if !lo.Contains(existUsers, w) {
				users = append(users, &model.User{Wallet: w})
			}
		}
	}

	ctx.JSON(http.StatusOK, api.Success(users))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Permission ------ ------

// GetFrontendPermission query frontend permission by user wallet
//
//	`GET /casbin?casbin_subject=0x1`
//
// @Summary Query frontend permission by user wallet
// @Tags Permission
// @Accept json
// @Produce json
// @Param casbin_subject query string true "casbin_subject"
// @Success 200 {object} Reply
// @Router /v1/casbin [get]
func GetFrontendPermission(ctx *gin.Context) {
	enforcer := api.ForContextOnlyEnforcer(ctx)

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
