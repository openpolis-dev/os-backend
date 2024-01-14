package api

import (
	"context"
	"fmt"
	"strconv"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal"
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
func ForContext(ctx *gin.Context) (user *middleware.CurUser, enforcer *casbin.SyncedEnforcer, db *gorm.DB, cfg *config.Config) {
	user, _ = ctx.Value(middleware.CurUserKey).(*middleware.CurUser)
	enforcer, _ = ctx.Value(middleware.EnforcerKey).(*casbin.SyncedEnforcer)
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

// ForContextOnlyPush read `Push` from `Context`
func ForContextOnlyPush(ctx *gin.Context) (push []sdk.Pusher) {
	push, _ = ctx.Value(middleware.PushKey).([]sdk.Pusher)

	return
}

// ForContextUserAndDB read `CurUser DB` from `Context`
func ForContextUserAndDB(ctx *gin.Context) (user *middleware.CurUser, db *gorm.DB) {
	user, _ = ctx.Value(middleware.CurUserKey).(*middleware.CurUser)
	db, _ = ctx.Value(middleware.DBKey).(*gorm.DB)

	return
}

// ForContextDBAndConfig read `DB Config` from `Context`
func ForContextDBAndConfig(ctx *gin.Context) (db *gorm.DB, cfg *config.Config) {
	db, _ = ctx.Value(middleware.DBKey).(*gorm.DB)
	cfg, _ = ctx.Value(middleware.CfgKey).(*config.Config)

	return
}

func GinContextFromContext(ctx context.Context) (*gin.Context, error) {
	ginContext := ctx.Value(middleware.GinCtxKey)
	if ginContext == nil {
		err := fmt.Errorf("could not retrieve gin.Context")
		return nil, err
	}

	gc, ok := ginContext.(*gin.Context)
	if !ok {
		err := fmt.Errorf("gin.Context has wrong type")
		return nil, err
	}
	return gc, nil
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ ------ ------ ------ ------ ------ ------ ------ ------

// ParseAndConvertPageParam parse and convert page param to `Page` from `Context`
func ParseAndConvertPageParam(ctx *gin.Context) *gormfind.Page {
	pageParam := ctx.Query("page")
	sizeParam := ctx.Query("size")
	sortField := ctx.Query("sort_field")
	sortOrder := ctx.Query("sort_order")

	if sortField == "" {
		sortField = "create_ts"
	}

	if sortOrder == "" {
		sortOrder = "desc"
	}

	page, _ := strconv.Atoi(pageParam)
	size, _ := strconv.Atoi(sizeParam)

	if page == 0 {
		page = 1
	}

	if size == 0 {
		size = internal.DefaultPageSize
	}

	return &gormfind.Page{
		Page:      page,
		Size:      size,
		SortField: &sortField,
		Order:     &sortOrder,
	}
}

func GetLangFromQuery(ctx *gin.Context, defaultLang string) string {
	return GetQueryParamsOrDefaultValue(ctx, "lang", "en")
}

func GetQueryParamsOrDefaultValue(ctx *gin.Context, paramKey string, defaultValue string) string {
	value, found := ctx.GetQuery(paramKey)
	if found {
		return value
	} else {
		return defaultValue
	}
}
