package publicity_inject

import (
	"errors"
	"fmt"
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
		publicityGroup = publicity.Gin.Group("/publicity")
		publicityAuthGroup = publicity.Gin.Group("/", middleware.AuthRequired).Group("/publicity")
	}

	publicityGroup.GET("/public", publicity.Public)

	publicityGroup.GET("/list", publicity.List)
	publicityGroup.GET("/detail/:id", publicity.Detail)

	publicityAuthGroup.POST("/create", publicity.Create)
	publicityAuthGroup.DELETE("/delete/:id", publicity.Delete)
	publicityAuthGroup.POST("/update/:id", publicity.Update)
}

func (c *PublicityController) Public(ctx *gin.Context) {
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

	sqlStr := "select p.id, p.creator, p.content, p.create_at, p.is_del, p.is_draft, p.season, p.title, p.update_at, u.avatar from publicities as p join users as u on p.creator = u.wallet where p.is_del = 0 and p.is_draft = 0"
	sqlStr += " order by p.create_at desc" + fmt.Sprintf(" limit %d offset %d", size, (page-1)*size)

	querySeg := c.Db.Raw(sqlStr)
	totalQuerySeg := c.Db.Table("publicities").Where("is_del = 0 and is_draft = 0")
	total, err := gormfind.Count(totalQuerySeg)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("total public list publicity error")))
		return
	}

	sortKey := "p.create_at"
	order := "desc"

	data, err := model.QueryRows[PublicityInfo](querySeg, &gormfind.Page{
		Page:      page,
		Size:      size,
		SortField: &sortKey,
		Order:     &order,
	})
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("public list publicity error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  page,
		Size:  size,
		Total: total,
		Rows:  data,
	}))
}

func (c *PublicityController) List(ctx *gin.Context) {
	pageParam := ctx.Query("page")
	sizeParam := ctx.Query("size")

	typeParam := ctx.Query("type")

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

	sqlStr := "select p.id, p.creator, p.content, p.create_at, p.is_del, p.is_draft, p.season, p.title, p.update_at, u.avatar from publicities as p join users as u on p.creator = u.wallet"

	if typeParam == "list" {
		sqlStr += " where p.is_del = 0 and p.is_draft = 0"
	} else if typeParam == "unlist" {
		sqlStr += " where p.is_del = 0 and p.is_draft = 1"
	} else if typeParam == "del" {
		sqlStr += " where p.is_del = 1"
	}

	sqlStr += " order by p.update_at desc" + fmt.Sprintf(" limit %d offset %d", size, (page-1)*size)

	querySeg := c.Db.Raw(sqlStr)
	totalQuerySeg := c.Db.Table("publicities")
	total, err := gormfind.Count(totalQuerySeg)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("total list publicity error")))
		return
	}

	sortKey := "p.update_at"
	order := "desc"

	data, err := model.QueryRows[PublicityInfo](querySeg, &gormfind.Page{
		Page:      page,
		Size:      size,
		SortField: &sortKey,
		Order:     &order,
	})
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list publicity error")))
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

	var data *PublicityInfo
	err = c.Db.Raw("select p.id, p.creator, p.content, p.create_at, p.is_del, p.is_draft, p.season, p.title, p.update_at, u.avatar from publicities as p join users as u on p.creator = u.wallet where p.id = ?", id).First(&data).Error
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get publicity error detail:"+err.Error())))
		return
	}

	var logs []*PublicityLogInfo
	err = c.Db.Raw("select p.id, p.eidtor, p.publicity_id, p.update_at, u.avatar from publicity_logs as p join users as u on p.eidtor = u.wallet where p.publicity_id = ?", id).Find(&logs).Error
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get publicity logs error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&PublicityDetail{
		Detail: data,
		Log:    logs,
	}))
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
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error detail:"+err.Error())))
		return
	}

	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	currentSeason, err := model.GetCurrentSeason(c.Db)
	if err != nil {
		log.Error().Msgf("fetch current season error: %v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("fetch current season error")))
		return
	}

	if req.IsSaveDraft {

		if req.ID > 0 {
			var data *model.Publicity
			err = c.Db.Model(&model.Publicity{}).Where("id = ? and is_del = 0 and is_draft = 1", req.ID).First(&data).Error
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get publicity error")))
				return
			}

			err = c.Db.Model(&model.Publicity{}).Where("id = ? and is_del = 0 and is_draft = 1", req.ID).
				Updates(&model.Publicity{
					Title:    req.Title,
					Content:  req.Content,
					UpdateAt: model.GetCurrentUtcEpochSecond(),
					Creator:  user.Wallet,
				}).Error
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("save publicity error")))
				return
			}

		} else {
			err = c.Db.Model(&model.Publicity{}).Create(&model.Publicity{
				Title:    req.Title,
				Content:  req.Content,
				CreateAt: 0,
				Creator:  user.Wallet,
				UpdateAt: model.GetCurrentUtcEpochSecond(),
				IsDel:    0,
				IsDraft:  1,
			}).Error
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("save publicity error")))
				return
			}
		}

	} else {
		if req.ID > 0 {
			var data *model.Publicity
			err = c.Db.Model(&model.Publicity{}).Where("id = ? and is_del = 0 and is_draft = 1", req.ID).First(&data).Error
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get publicity error")))
				return
			}

			err = c.Db.Model(&model.Publicity{}).Where("id = ? and is_del = 0 and is_draft = 1", req.ID).
				Updates(map[string]interface{}{
					"title":     req.Title,
					"content":   req.Content,
					"update_at": model.GetCurrentUtcEpochSecond(),
					"creator":   user.Wallet,
					"is_draft":  0,
					"create_at": model.GetCurrentUtcEpochSecond(),
					"season":    int(currentSeason.Idx),
				}).Error
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("save publicity error")))
				return
			}

		} else {
			err = c.Db.Model(&model.Publicity{}).Create(&model.Publicity{
				Title:    req.Title,
				Content:  req.Content,
				CreateAt: model.GetCurrentUtcEpochSecond(),
				Creator:  user.Wallet,
				UpdateAt: model.GetCurrentUtcEpochSecond(),
				IsDel:    0,
				Season:   int(currentSeason.Idx),
				IsDraft:  0,
			}).Error
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create publicity error")))
				return
			}
		}
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

	currentSeason, err := model.GetCurrentSeason(c.Db)
	if err != nil {
		log.Error().Msgf("fetch current season error: %v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("fetch current season error")))
		return
	}

	err = c.Db.Model(&model.Publicity{}).Where("id = ? and (season = ? or is_draft = 1) and is_del = 0", id, currentSeason.Idx).Update("is_del", 1).Error
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("delete publicity error detail:"+err.Error())))
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
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error detail:"+err.Error())))
		return
	}

	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	currentSeason, err := model.GetCurrentSeason(c.Db)
	if err != nil {
		log.Error().Msgf("fetch current season error: %v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("fetch current season error")))
		return
	}

	var data *model.Publicity
	err = c.Db.Model(&model.Publicity{}).Where("id = ? and is_del = 0 and season = ?", req.ID, currentSeason.Idx).First(&data).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get publicity error")))
		return
	}

	err = c.Db.Model(data).
		Updates(&model.Publicity{
			Title:    req.Title,
			Content:  req.Content,
			UpdateAt: model.GetCurrentUtcEpochSecond(),
		}).Error
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update publicity error detail:"+err.Error())))
		return
	} else {
		err = c.Db.Model(&model.PublicityLog{}).Create(&model.PublicityLog{
			UpdateAt:    model.GetCurrentUtcEpochSecond(),
			Eidtor:      user.Wallet,
			PublicityID: data.ID,
		}).Error
		if err != nil {
			log.Error().Msgf("add publicity editor log error: %v", err)
			sdk.LogServerErrorToSentry(ctx, err)
		}
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}
