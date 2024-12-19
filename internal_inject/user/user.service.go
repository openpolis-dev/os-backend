package user_inject

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/spruceid/siwe-go"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"

	eth_common "github.com/ethereum/go-ethereum/common"
	unipass_sigverify "github.com/unipassid/unipass-sigverify-go"
)

type UserService struct {
	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`
}

func (u *UserService) RefreshNonce(ctx *gin.Context, wallet string, userNonce *model.UserNonce) error {
	txErr := u.Db.Transaction(func(tx *gorm.DB) error {
		// new nonce and refreshAt
		nonce := siwe.GenerateNonce()
		refreshAt := time.Now().UnixMilli()
		// update with new value
		if userNonce == nil {
			userNonce = &model.UserNonce{Wallet: common.FormatUserWallet(wallet)}
		}
		userNonce.Nonce = nonce
		userNonce.RefreshAt = refreshAt
		err := model.UserNonceModel.CreateOrUpdate(tx, userNonce)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			return err
		}

		return nil
	})

	return txErr
}

func (u *UserService) Login(ctx *gin.Context, req *LoginReq) (int, *api.Reply) {
	var token string
	var tokenExp int64
	var user *model.User
	var httpCode int
	var reply *api.Reply

	userVerified := true

	_ = u.Db.Transaction(func(tx *gorm.DB) error {

		// verify sign
		// --> query nonce
		userNonce, err := model.UserNonceModel.RecentNonce(tx, common.FormatUserWallet(req.Wallet), u.Cfg.Auth.NonceLifespan)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("nonce not found")))
			httpCode = http.StatusInternalServerError
			reply = api.ServerError(errors.New("nonce not found"))

			return errors.New("nonce not found")
		}
		if userNonce == nil {
			// ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("please refresh nonce firstly")))
			httpCode = http.StatusBadRequest
			reply = api.BadRequest(errors.New("please refresh nonce firstly"))

			return errors.New("please refresh nonce firstly")
		}
		if strings.EqualFold(req.WalletType, "EOA") {
			// --> verify signature
			message, err := siwe.ParseMessage(req.Message)
			if err != nil {
				// ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
				httpCode = http.StatusBadRequest
				reply = api.BadRequest(err)

				return err
			}
			timestamp := time.Now()
			publicKey, err := message.Verify(req.Signature, &req.Domain, &userNonce.Nonce, &timestamp)
			if err != nil {
				// ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
				httpCode = http.StatusBadRequest
				reply = api.BadRequest(err)

				return err
			}
			if !strings.EqualFold(req.Wallet, crypto.PubkeyToAddress(*publicKey).Hex()) {
				// ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("signature not match")))
				httpCode = http.StatusBadRequest
				reply = api.BadRequest(errors.New("signature not match"))

				return errors.New("signature not match")
			}
		} else if strings.EqualFold(req.WalletType, "AA") {
			client, err := ethclient.Dial(u.Cfg.Auth.PolygonRPC)
			if err != nil {
				sdk.LogServerErrorToSentry(ctx, err)
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("failed to connect to polygon rpc")))
				httpCode = http.StatusInternalServerError
				reply = api.ServerError(errors.New("failed to connect to polygon rpc"))

				return errors.New("failed to connect to polygon rpc")
			}

			// get AA's bytecode
			bytecode, err := client.CodeAt(context.Background(), eth_common.HexToAddress(req.Wallet), nil)
			if err != nil {
				sdk.LogServerErrorToSentry(ctx, err)
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("failed to get bytecode")))
				httpCode = http.StatusInternalServerError
				reply = api.ServerError(errors.New("failed to get bytecode"))

				return errors.New("failed to get bytecode")
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
					// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("failed to verify signature")))
					httpCode = http.StatusInternalServerError
					reply = api.ServerError(errors.New("failed to verify signature"))

					return errors.New("failed to verify signature")
				}
				if !ok {
					// ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("signature not match")))
					httpCode = http.StatusBadRequest
					reply = api.BadRequest(errors.New("signature not match"))

					return errors.New("signature not match")
				}
			} else {
				userVerified = false
			}
		}

		// query user
		user, err = model.UserModel.Detail(tx, common.FormatUserWallet(req.Wallet))
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("user not found")))
			httpCode = http.StatusInternalServerError
			reply = api.ServerError(errors.New("user not found"))

			return errors.New("user not found")
		}
		if user == nil {
			user = &model.User{
				Wallet: common.FormatUserWallet(req.Wallet),
			}
			err = model.UserModel.CreateOrUpdate(tx, user)
			if err != nil {
				sdk.LogServerErrorToSentry(ctx, err)
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("failed to create user")))
				httpCode = http.StatusInternalServerError
				reply = api.ServerError(errors.New("failed to create user"))

				return errors.New("failed to create user")
			}
		}

		// generate jwt token
		token, tokenExp, err = common.GenerateJwtToken[middleware.CurUser](
			&middleware.CurUser{
				Wallet: user.Wallet,
			},
			time.Duration(u.Cfg.Jwt.Exp)*time.Hour,
			u.Cfg.Jwt.Secret,
		)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("failed to generate jwt token")))
			httpCode = http.StatusInternalServerError
			reply = api.ServerError(errors.New("failed to generate jwt token"))

			return errors.New("failed to generate jwt token")
		}

		httpCode = http.StatusOK
		reply = api.Success(&LoginReply{
			Token:        token,
			TokenExp:     tokenExp,
			User:         user,
			UserVerified: userVerified,
		})

		return nil
	})

	return httpCode, reply
}

func (u *UserService) Update(ctx *gin.Context, user *middleware.CurUser, req *UpdateReq) (int, *api.Reply) {
	userM, err := model.UserModel.Detail(u.Db, user.Wallet)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("user not found")))
		return http.StatusInternalServerError, api.ServerError(errors.New("user not found"))
	}
	if u == nil {
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("user %s not found", user.Wallet)))
		return http.StatusBadRequest, api.BadRequest(fmt.Errorf("user %s not found", user.Wallet))
	}

	// update user info
	userM.Name = req.Name
	userM.Bio = req.Bio
	userM.Email = req.Email
	userM.Wechat = req.Wechat
	userM.DiscordProfile = req.DiscordProfile
	userM.TwitterProfile = req.TwitterProfile
	userM.GoogleProfile = req.GoogleProfile
	userM.GithubProfile = req.GithubProfile
	userM.Mirror = req.Mirror

	// Only upload image when data is b64 image string (start with `data:image`)
	avatarUrl, err := sdk.GetAwsClient().UploadUserAvatar(userM.Wallet, req.Avatar)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("upload avatar error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("upload avatar error"))
	}
	userM.Avatar = avatarUrl

	err = model.UserModel.CreateOrUpdate(u.Db, userM)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update user error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("update user error"))
	}

	sppClient := sdk.GetSppClient()
	sppUpdatePayload := userM.BuildSppUpdateProfilePayload()
	err = sppClient.UpdateProfile(user.Wallet, sppUpdatePayload)
	if err != nil {
		log.Error().Msgf("update user %s info to spp error, update req data: %+v, error: %+v", user.Wallet, req, err)
	}

	return http.StatusOK, api.Success(nil)
}
