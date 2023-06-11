package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
)

const BearerSchema = "Bearer"

const CurUserKey = "u"

type CurUser struct {
	Wallet string
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
