package seeauth_inject

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	user_inject "github.com/theseed-labs/os-backend/internal_inject/user"
	"gorm.io/gorm"

	seeauth "github.com/Taoist-Labs/see-auth-go"
	"github.com/Taoist-Labs/see-auth-go/proof"
)

type SeeAuthService struct {
	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`
}

func (s *SeeAuthService) SeeAuthNonce(ctx *gin.Context, wallet string) (int, *api.Reply) {
	userNonce, err := UserNonceModel.Detail(s.Db, common.FormatUserWallet(wallet))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("nonce not found")))
		return http.StatusInternalServerError, api.ServerError(errors.New("nonce not found"))
	}

	// new nonce and refreshAt
	nonce := seeauth.GenerateNonce() // DIFFERENT with `RefreshNonce` api
	refreshAt := time.Now().UnixMilli()
	// update with new value
	if userNonce == nil {
		userNonce = &UserNonce{Wallet: common.FormatUserWallet(wallet)}
	}
	userNonce.Nonce = nonce
	userNonce.RefreshAt = refreshAt
	err = UserNonceModel.CreateOrUpdate(s.Db, userNonce)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update nonce error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("update nonce error"))
	}

	return http.StatusOK, api.Success(&RefreshNonceReply{Nonce: nonce})
}

func (s *SeeAuthService) LoginWithSeeAuth(ctx *gin.Context, req *seeauth.SeeLogin) (int, *api.Reply) {
	userNonce, err := UserNonceModel.RecentNonce(s.Db, common.FormatUserWallet(req.Wallet), s.Cfg.Auth.NonceLifespan)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("nonce not found")))
		return http.StatusInternalServerError, api.ServerError(errors.New("nonce not found"))
	}
	if userNonce == nil {
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("please refresh nonce firstly")))
		return http.StatusBadRequest, api.BadRequest(errors.New("please refresh nonce firstly"))
	}

	seeAuthPk, err := model.GetSeeAuthPk(s.Db)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("get see auth private key error: %+v, contract admin", err)))
		return http.StatusInternalServerError, api.ServerError(fmt.Errorf("get see auth private key error: %+v, contract admin", err))
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
		PrivateKey: seeAuthPk,
	})
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return http.StatusInternalServerError, api.ServerError(err)
	}

	// query user
	user, err := user_inject.UserModel.Detail(s.Db, common.FormatUserWallet(req.Wallet))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("user not found")))
		return http.StatusInternalServerError, api.ServerError(errors.New("user not found"))
	}
	if user == nil {
		user = &user_inject.User{
			Wallet: common.FormatUserWallet(req.Wallet),
		}
		err = user_inject.UserModel.CreateOrUpdate(s.Db, user)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("failed to create user")))
			return http.StatusInternalServerError, api.ServerError(errors.New("failed to create user"))
		}
	}

	// generate jwt token
	token, tokenExp, err := common.GenerateJwtToken[middleware.CurUser](
		&middleware.CurUser{
			Wallet: user.Wallet,
		},
		time.Duration(s.Cfg.Jwt.Exp)*time.Hour,
		s.Cfg.Jwt.Secret,
	)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("failed to generate jwt token")))
		return http.StatusInternalServerError, api.ServerError(errors.New("failed to generate jwt token"))
	}

	return http.StatusOK, api.Success(&LoginWithSeeAuthReply{
		Token:    token,
		TokenExp: tokenExp,
		User:     user,
		SEEAuth:  seeAuth,
	})
}
