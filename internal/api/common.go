package api

import (
	"strconv"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/xiaosongfu/gormfind"
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

// ForContext read `CurUser DB Config` from `Context`
func ForContext(ctx *gin.Context) (user *middleware.CurUser, enforcer *casbin.Enforcer, db *gorm.DB, cfg *config.Config) {
	user, _ = ctx.Value(middleware.CurUserKey).(*middleware.CurUser)
	enforcer, _ = ctx.Value(middleware.EnforcerKey).(*casbin.Enforcer)
	db, _ = ctx.Value(middleware.DBKey).(*gorm.DB)
	cfg, _ = ctx.Value(middleware.CfgKey).(*config.Config)

	return
}

// ForContextOnlyUser read only `CurUser` from `Context`
func ForContextOnlyUser(ctx *gin.Context) (user *middleware.CurUser) {
	user, _ = ctx.Value(middleware.CurUserKey).(*middleware.CurUser)

	return
}

// ForContextOnlyDB read only `DB` from `Context`
func ForContextOnlyDB(ctx *gin.Context) (db *gorm.DB) {
	db, _ = ctx.Value(middleware.DBKey).(*gorm.DB)

	return
}

// ForContextUserAndDB read  `CurUser DB` from `Context`
func ForContextUserAndDB(ctx *gin.Context) (user *middleware.CurUser, db *gorm.DB) {
	user, _ = ctx.Value(middleware.CurUserKey).(*middleware.CurUser)
	db, _ = ctx.Value(middleware.DBKey).(*gorm.DB)

	return
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ ------ ------ ------ ------ ------ ------ ------ ------

// ParseAndConvertPageParam parse and convert page param to `Page` from `Context`
func ParseAndConvertPageParam(ctx *gin.Context) *gormfind.Page {
	pageParam := ctx.Query("page")
	sizeParam := ctx.Query("size")
	sortField := ctx.Query("sort_field")
	sortOrder := ctx.Query("sort_order")

	page, _ := strconv.Atoi(pageParam)
	size, _ := strconv.Atoi(sizeParam)

	return &gormfind.Page{
		Page:      page,
		Size:      size,
		SortField: &sortField,
		Order:     &sortOrder,
	}
}
