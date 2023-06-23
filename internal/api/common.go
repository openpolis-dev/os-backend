package api

import (
	"strconv"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type Reply struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
}

type ListReplyData struct {
	Page  int   `json:"page"`
	Size  int   `json:"size"`
	Total int64 `json:"total"`
	Rows  any   `json:"rows,omitempty"`
}

func Success(data any) *Reply {
	return &Reply{
		Code: 200,
		Msg:  "OK",
		Data: data,
	}
}

func BadRequest(err error) *Reply {
	return &Reply{
		Code: -1,
		Msg:  err.Error(),
	}
}

func ServerError(err error) *Reply {
	return &Reply{
		Code: -1,
		Msg:  err.Error(),
	}
}

func Forbidden() *Reply {
	return &Reply{
		Code: -1,
		Msg:  "Forbidden",
	}
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ ------ ------ ------ ------ ------ ------ ------ ------

// ForContext read `CurUser Enforcer DB Config` from `Context`
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

// ForContextOnlyEnforcer read `Enforcer` from `Context`
func ForContextOnlyEnforcer(ctx *gin.Context) (enforcer *casbin.Enforcer) {
	enforcer, _ = ctx.Value(middleware.EnforcerKey).(*casbin.Enforcer)

	return
}

// ForContextOnlyDB read only `DB` from `Context`
func ForContextOnlyDB(ctx *gin.Context) (db *gorm.DB) {
	db, _ = ctx.Value(middleware.DBKey).(*gorm.DB)

	return
}

// ForContextOnlyNotificator read `Notificator` from `Context`
func ForContextOnlyNotificator(ctx *gin.Context) (notificator sdk.Notificator) {
	notificator, _ = ctx.Value(middleware.NotificatorKey).(sdk.Notificator)

	return
}

// ForContextUserAndDB read `CurUser DB` from `Context`
func ForContextUserAndDB(ctx *gin.Context) (user *middleware.CurUser, db *gorm.DB) {
	user, _ = ctx.Value(middleware.CurUserKey).(*middleware.CurUser)
	db, _ = ctx.Value(middleware.DBKey).(*gorm.DB)

	return
}

// ForContextUserAndNotificator read `Notificator` from `Context`
func ForContextUserAndNotificator(ctx *gin.Context) (user *middleware.CurUser, notificator sdk.Notificator) {
	user, _ = ctx.Value(middleware.CurUserKey).(*middleware.CurUser)
	notificator, _ = ctx.Value(middleware.NotificatorKey).(sdk.Notificator)

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
