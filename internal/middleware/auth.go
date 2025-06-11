package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
)

const BearerSchema = "Bearer"

const CurUserKey = "u"

type CurUser struct {
	Wallet string
}

func AuthOption(ctx *gin.Context) {
	// `Authorization: Bearer <token>`
	authHeader := ctx.GetHeader("Authorization")
	log.Debug().Msgf("auth option: header: %+v", authHeader)
	if len(authHeader) > len(BearerSchema) {
		token := authHeader[len(BearerSchema)+1:]

		cfg, _ := ctx.Value(CfgKey).(*config.Config)
		user, err := common.ValidateJwtToken[CurUser](token, cfg.Jwt.Secret)
		if err == nil {
			ctx.Set(CurUserKey, user)
		}
	}

	// <-- before
	ctx.Next()
	// --> after
}

func AuthRequired(ctx *gin.Context) {
	// `Authorization: Bearer <token>`
	authHeader := ctx.GetHeader("Authorization")
	if authHeader == "" || len(authHeader) < len(BearerSchema) {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	token := authHeader[len(BearerSchema)+1:]

	cfg, _ := ctx.Value(CfgKey).(*config.Config)
	user, err := common.ValidateJwtToken[CurUser](token, cfg.Jwt.Secret)
	if err != nil {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	ctx.Set(CurUserKey, user)

	// <-- before
	ctx.Next()
	// --> after
}

func AdminPermissionRequired(ctx *gin.Context) {
	cfg := ctx.Value(CfgKey).(*config.Config)

	// check whether the admin token is existing, if not, always return 401
	if cfg.Admin.AuthToken == "" {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	authHeader := ctx.GetHeader("AdminAuth")
	if authHeader != cfg.Admin.AuthToken {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	ctx.Next()
}
