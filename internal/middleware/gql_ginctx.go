package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
)

func GinContextToContextMiddleware(ctx *gin.Context) {
	c := context.WithValue(ctx.Request.Context(), GinCtxKey, ctx)
	ctx.Request = ctx.Request.WithContext(c)

	ctx.Next()
}
