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

	seeauth "github.com/Taoist-Labs/see-auth-go"
	"github.com/Taoist-Labs/see-auth-go/proof"
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

// UserModelWithSomeSeepassData is a temporary solution for returning user data with some seepass data struct such as sb, seed and social network accounts.
// The new struct here is to keep both old structure and new added seepass data.
type UserModelWithSomeSeepassData struct {
	model.User
	Sp *sdk.SeepassResponse `json:"sp"`
}

// RefreshNonce refresh nonce
//
//	@Summary	Refresh nonce
//	@Tags		Auth
//	@Accept		json
//	@Produce	json
//	@Param		JsonBody	body		RefreshNonceReq	true	"request json body"
//	@Success	200			{object}	api.Reply{data=RefreshNonceReply}
//	@Router		/user/refresh_nonce [post]
func RefreshNonce(ctx *gin.Context) {
	req := RefreshNonceReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	db := api.ForContextOnlyDB(ctx)

	userNonce, err := model.UserNonceModel.Detail(db, common.FormatUserWallet(req.Wallet))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("nonce not found")))
		return
	}

	// new nonce and refreshAt
	nonce := siwe.GenerateNonce()
	refreshAt := time.Now().UnixMilli()
	// update with new value
	if userNonce == nil {
		userNonce = &model.UserNonce{Wallet: common.FormatUserWallet(req.Wallet)}
	}
	userNonce.Nonce = nonce
	userNonce.RefreshAt = refreshAt
	err = model.UserNonceModel.CreateOrUpdate(db, userNonce)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update nonce error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&RefreshNonceReply{Nonce: nonce}))
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

	UserVerified bool `json:"user_verified"` // for unipass user,if wallet signature not verified, will be false
}

// Login user login
//
//	@Summary	Login
//	@Tags		Auth
//	@Accept		json
//	@Produce	json
//	@Param		JsonBody	body		LoginReq	true	"request json body"
//	@Success	200			{object}	api.Reply{data=LoginReply}
//	@Router		/user/login [post]
func Login(ctx *gin.Context) {
	req := LoginReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	_, _, db, cfg := api.ForContext(ctx)

	// user verified flag
	userVerified := true

	// verify sign
	// --> query nonce
	userNonce, err := model.UserNonceModel.RecentNonce(db, common.FormatUserWallet(req.Wallet), cfg.Auth.NonceLifespan)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("nonce not found")))
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
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("failed to connect to polygon rpc")))
			return
		}

		// get AA's bytecode
		bytecode, err := client.CodeAt(context.Background(), eth_common.HexToAddress(req.Wallet), nil)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("failed to get bytecode")))
			return
		}
		// if bytecode is not empty, means the wallet has deployed
		// 2023/12/04: only verify signature when AA is deployed!
		if len(bytecode) > 0 {
			account := eth_common.HexToAddress(req.Wallet)
			sig := eth_common.FromHex(req.Signature)
			msg := []byte(req.Message)

			ok, err := unipass_sigverify.VerifyMessageSignature(context.Background(), account, msg, sig, req.IsEIP191Prefix, client)
			if err != nil {
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("failed to verify signature")))
				return
			}
			if !ok {
				ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("signature not match")))
				return
			}
		} else {
			userVerified = false
		}
	}

	// query user
	user, err := model.UserModel.Detail(db, common.FormatUserWallet(req.Wallet))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("user not found")))
		return
	}
	if user == nil {
		user = &model.User{
			Wallet: common.FormatUserWallet(req.Wallet),
		}
		err = model.UserModel.CreateOrUpdate(db, user)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("failed to create user")))
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
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("failed to generate jwt token")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&LoginReply{
		Token:        token,
		TokenExp:     tokenExp,
		User:         user,
		UserVerified: userVerified,
	}))
}

// Logout user logout
//
//	@Summary	Logout
//	@Tags		Auth
//	@Accept		json
//	@Produce	json
//	@Success	200	{object}	api.Reply
//	@Router		/user/logout [post]
func Logout(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, api.Success(nil))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ SeeAuth ------ ------

// SeeAuthNonce SeeAuth nonce
//
//	@Summary	get nonce of SeeAuth
//	@Tags		SeeAuth
//	@Accept		json
//	@Produce	json
//	@Param		wallet	    path		string	true	"request json body"
//	@Success	200			{object}	api.Reply{data=RefreshNonceReply}
//	@Router		/seeauth/nonce/:wallet [get]
func SeeAuthNonce(ctx *gin.Context) {
	wallet := ctx.Param("wallet") // DIFFERENT with `RefreshNonce` api

	db := api.ForContextOnlyDB(ctx)

	userNonce, err := model.UserNonceModel.Detail(db, common.FormatUserWallet(wallet))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("nonce not found")))
		return
	}

	// new nonce and refreshAt
	nonce := seeauth.GenerateNonce() // DIFFERENT with `RefreshNonce` api
	refreshAt := time.Now().UnixMilli()
	// update with new value
	if userNonce == nil {
		userNonce = &model.UserNonce{Wallet: common.FormatUserWallet(wallet)}
	}
	userNonce.Nonce = nonce
	userNonce.RefreshAt = refreshAt
	err = model.UserNonceModel.CreateOrUpdate(db, userNonce)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update nonce error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&RefreshNonceReply{Nonce: nonce}))
}

type LoginWithSeeAuthReply struct {
	Token    string           `json:"token"`
	TokenExp int64            `json:"token_exp"` // time unit: seconds
	User     *model.User      `json:"user"`
	SEEAuth  *seeauth.SeeAuth `json:"see_auth"`
}

// LoginWithSeeAuth user login
//
//	@Summary	Login
//	@Tags		SeeAuth
//	@Accept		json
//	@Produce	json
//	@Param		JsonBody	body		LoginReq	true	"request json body"
//	@Success	200			{object}	api.Reply{data=LoginWithSeeAuthReply}
//	@Router		/seeauth/login [post]
func LoginWithSeeAuth(ctx *gin.Context) {
	req := seeauth.SeeLogin{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	_, _, db, cfg := api.ForContext(ctx)

	// query nonce
	userNonce, err := model.UserNonceModel.RecentNonce(db, common.FormatUserWallet(req.Wallet), cfg.Auth.NonceLifespan)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("nonce not found")))
		return
	}
	if userNonce == nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("please refresh nonce firstly")))
		return
	}

	// SEE-AUTH Logic
	seeAuth, err := seeauth.Auth(&seeauth.SignatureParams{
		WalletName: req.WalletName,
		Wallet:     req.Wallet,
		Domain:     req.Domain,
		Nonce:      userNonce.Nonce,
		Message:    req.Message,
		Signature:  req.Signature,
	}, &seeauth.ProofParams{
		Recipient: "0x0000000000000000000000000000000000000000",
		Schema: &proof.SchemaData{
			Wallet: req.Wallet,
			Vendor: "common",
		},
		PrivateKey: "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d",
	})
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	// query user
	user, err := model.UserModel.Detail(db, common.FormatUserWallet(req.Wallet))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("user not found")))
		return
	}
	if user == nil {
		user = &model.User{
			Wallet: common.FormatUserWallet(req.Wallet),
		}
		err = model.UserModel.CreateOrUpdate(db, user)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("failed to create user")))
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
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("failed to generate jwt token")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&LoginWithSeeAuthReply{
		Token:    token,
		TokenExp: tokenExp,
		User:     user,
		SEEAuth:  seeAuth,
	}))
}

// SeeAuthTestApi SeeAuth test api
func SeeAuthTestApi(ctx *gin.Context) {
	req := seeauth.SeeAuth{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	wallet, err := seeauth.SeeDAOAuth("0x0000000000000000000000000000000000000000", &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, struct {
		Wallet string `json:"wallet"`
		Token  string `json:"token"`
	}{
		Wallet: wallet,
		Token:  "test.jwt.token",
	})
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ User ------ ------

// Detail get user detail
//
//	@Summary	Get user detail
//	@Tags		User
//	@Accept		json
//	@Produce	json
//	@Success	200	{object}	api.Reply{data=sdk.SeepassResponse}
//	@Router		/user/me [get]
func Detail(ctx *gin.Context) {
	// TODO: Get data from seepass API, and return data from DB if seepass returns 404
	user, db := api.ForContextUserAndDB(ctx)

	sppClient := sdk.GetSppClient()
	seepassResp, err := sppClient.GetSeepassData(user.Wallet)
	if err == nil {
		ctx.JSON(http.StatusOK, api.Success(seepassResp))
		return
	}

	log.Warn().Msgf("query seepass data error, wallet: %s, error: %+v", user.Wallet, err)

	u, err := model.UserModel.Detail(db, user.Wallet)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("user not found")))
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

	// TODO: Move the hardcoded data to some const data or configuration service
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
	GithubProfile  string `json:"github_profile"`
	Mirror         string `json:"mirror"`
}

// Update `PUT /me`
//
//	@Summary	Update user info
//	@Tags		User
//	@Accept		json
//	@Produce	json
//	@Param		JsonBody	body		UpdateReq	true	"request json body"
//	@Success	200			{object}	api.Reply
//	@Router		/user/me [put]
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
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("user not found")))
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
	u.GithubProfile = req.GithubProfile
	u.Mirror = req.Mirror

	// Only upload image when data is b64 image string (start with `data:image`)
	avatarUrl, err := sdk.GetAwsClient().UploadUserAvatar(u.Wallet, req.Avatar)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("upload avatar error")))
		return
	}
	u.Avatar = avatarUrl

	err = model.UserModel.CreateOrUpdate(db, u)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update user error")))
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
//	@Summary	Query multiple users by wallet array on batch
//	@Tags		User
//	@Accept		json
//	@Produce	json
//	@Param		wallets	query		[]string	true	"wallets"
//	@Success	200		{object}	api.Reply{data=[]UserModelWithSomeSeepassData}
//	@Router		/user/users [get]
func Users(ctx *gin.Context) {
	wallets := ctx.QueryArray("wallets")
	//// convert all wallet to lower case
	//wallets := lo.Map[string](walletsParam, func(item string, _ int) string {
	//	return strings.ToLower(item)
	//})

	db := api.ForContextOnlyDB(ctx)
	sppClient := sdk.GetSppClient()

	users, err := model.UserModel.List(db, wallets)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query users error")))
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

	var rslt []UserModelWithSomeSeepassData

	// TODO: Query SeePASS to get user SBT and SEED info
	for _, user := range users {
		seepassResp, err := sppClient.GetSeepassData(user.Wallet)
		user.Wallet = common.ToFrontendWallet(user.Wallet)
		if err != nil {
			log.Warn().Msgf("query seepass data error, wallet: %s, error: %+v", user.Wallet, err)
		}
		if seepassResp != nil {
			rslt = append(rslt, UserModelWithSomeSeepassData{
				*user,
				seepassResp,
			})
		} else {
			rslt = append(rslt, UserModelWithSomeSeepassData{
				*user,
				nil,
			})
		}

	}

	ctx.JSON(http.StatusOK, api.Success(rslt))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Permission ------ ------

// GetFrontendPermission query frontend permission by user wallet
//
//	`GET /casbin?casbin_subject=0x1`
//
//	@Summary	Query frontend permission by user wallet
//	@Tags		Permission
//	@Accept		json
//	@Produce	json
//	@Param		casbin_subject	query		string	true	"casbin_subject"
//	@Success	200				{object}	api.Reply{data=string}
//	@Router		/user/casbin [get]
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
			if eth_common.IsHexAddress(role[i][0]) {
				role[i][0] = common.ToFrontendWallet(role[i][0])
			}
			policies = append(policies, append([]string{ptype}, role[i]...))
		}
	}
	m["p"] = policies
	result := bytes.NewBuffer([]byte{})
	encoder := json.NewEncoder(result)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(m)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query frontend permission error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(result.String()))
}
