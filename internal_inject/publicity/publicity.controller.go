package publicity_inject

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type PublicityController struct {
	// inject
	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	PublicityService *PublicityService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var publicity PublicityController

	err := inject.Populate(&publicity, &PublicityService{}, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var publicityGroup *gin.RouterGroup
	var publicityAuthGroup *gin.RouterGroup

	if fatherGroup != nil {
		publicityGroup = fatherGroup.Group("/publicity")
		publicityAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/publicity")
	} else {
		publicityGroup = publicity.Gin.Group("/projects")
		publicityAuthGroup = publicity.Gin.Group("/", middleware.AuthRequired).Group("/publicity")
	}

	publicityGroup.GET("/list", publicity.List)
	publicityGroup.GET("/detail/:id", publicity.Detail)

	publicityAuthGroup.POST("/create", publicity.Create)
	publicityAuthGroup.DELETE("/delete/:id", publicity.Delete)
	publicityAuthGroup.PUT("/update/:id", publicity.Update)
}

func (c *PublicityController) List(ctx *gin.Context) {
	pageParam := ctx.Query("page")
	sizeParam := ctx.Query("size")

	page, err := strconv.Atoi(pageParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	size, err := strconv.Atoi(sizeParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	querySeg := c.Db.Table("publicities").Where("is_del = 0")

	total, err := gormfind.Count(querySeg)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list projects error")))
		return
	}

	sortKey := "create_time"
	order := "desc"

	data, err := model.QueryRows[model.Publicity](querySeg, &gormfind.Page{
		Page:      page,
		Size:      size,
		SortField: &sortKey,
		Order:     &order,
	})
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list projects error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  page,
		Size:  size,
		Total: total,
		Rows:  data,
	}))
}

func (c *PublicityController) Detail(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	var data *model.Publicity
	err = c.Db.Model(&model.Publicity{}).Where("id = ? and is_del = 0", id).First(&data).Error
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get publicity error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(data))
}

func (c *PublicityController) Create(ctx *gin.Context) {
	req := CreateReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		log.Error().Msgf("Parse request error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	user, enforcer, _, _ := api.ForContext(ctx)

	//  check permission
	ok, err := enforcer.HasRoleForUser(user.Wallet, internal.RoleHall)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error")))
		return
	}

	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	err = c.Db.Model(&model.Publicity{}).Create(&model.Publicity{
		Title:    req.Title,
		Content:  req.Content,
		CreateAt: model.GetCurrentUtcEpochSecond(),
		Creator:  user.Wallet,
		UpdateAt: model.GetCurrentUtcEpochSecond(),
		IsDel:    0,
		Eidtor:   user.Wallet,
	}).Error
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create publicity error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func (c *PublicityController) Delete(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	err = c.Db.Model(&model.Publicity{}).Where("id = ?", id).Update("is_del", 1).Error
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("delete publicity error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func (c *PublicityController) Update(ctx *gin.Context) {
	req := UpdateReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		log.Error().Msgf("Parse request error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	user, enforcer, _, _ := api.ForContext(ctx)

	//  check permission
	ok, err := enforcer.HasRoleForUser(user.Wallet, internal.RoleHall)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error")))
		return
	}

	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	var data *model.Publicity
	err = c.Db.Model(&model.Publicity{}).Where("id = ? and is_del = 0", id).First(&data).Error
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get publicity error")))
		return
	}

	err = c.Db.Model(data).
		Update("title", req.Title).
		Update("content", req.Content).
		Update("update_at", model.GetCurrentUtcEpochSecond()).
		Update("editor", user.Wallet).Error
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create publicity error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}
