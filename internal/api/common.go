package api

import (
	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"gorm.io/gorm"
)

type Reply struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
}

func Success(data any) *Reply {
	return &Reply{
		Code: 200,
		Msg:  "OK",
		Data: data,
	}
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ ------ ------ ------ ------ ------ ------ ------ ------

// ForContext 从 context 中查找 UserData
func ForContext(ctx *gin.Context) (user *middleware.CurUser, db *gorm.DB, cfg *config.Config) {
	user, _ = ctx.Value(middleware.CurUserKey).(*middleware.CurUser)
	db, _ = ctx.Value(middleware.DBKey).(*gorm.DB)
	cfg, _ = ctx.Value(middleware.CfgKey).(*config.Config)

	return
}
