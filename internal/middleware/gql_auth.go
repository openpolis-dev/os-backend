package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
)

func GqlAuth(ctx *gin.Context) {
	// `Authorization: Bearer <token>`
	authHeader := ctx.GetHeader("Authorization")
	if authHeader != "" || len(authHeader) >= len(BearerSchema) {
		token := authHeader[len(BearerSchema)+1:]

		cfg, _ := ctx.Value(CfgKey).(*config.Config)
		user, err := common.ValidateJwtToken[CurUser](token, cfg.Jwt.Secret)
		if err != nil {
			ctx.String(http.StatusUnauthorized, `{ "errors": [ { "message": "unauthorized" } ], "data": null }`)
			return
		}

		ctx.Set(CurUserKey, user)
	}

	ctx.Next()
}
