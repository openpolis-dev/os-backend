package commonbudgetsources_inject

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type CommonBudgetSourcesController struct {
	// inject

	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var commonBudgetSources CommonBudgetSourcesController

	err := inject.Populate(&commonBudgetSources, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var commonBudgetSourceGroup *gin.RouterGroup

	if fatherGroup != nil {
		commonBudgetSourceGroup = fatherGroup.Group("/common_budget_sources")
	} else {
		commonBudgetSourceGroup = commonBudgetSources.Gin.Group("/common_budget_sources")
	}

	// no auth
	commonBudgetSourceGroup.GET("/", commonBudgetSources.List)
}

func (c *CommonBudgetSourcesController) List(ctx *gin.Context) {
	page := api.ParseAndConvertPageParam(ctx)

	keywords := ctx.Query("keywords")
	wallet := ctx.Query("wallet")
	openOnly := ctx.Query("open_only")

	querySeg := c.Db.Model(&model.CommonBudgetSource{})
	if keywords != "" {
		querySeg.Where(fmt.Sprintf("name ILIKE '%%%s%%'", keywords))
	}
	if wallet != "" {
		w := fmt.Sprintf("%%\"%s\"%%", wallet) // value is: `%0x123%`
		querySeg.Where(fmt.Sprintf("sponsors::text ILIKE '%%%s%%'", w))
	}

	if openOnly == "true" {
		querySeg = querySeg.Where("state=open")
	}

	total, err := gormfind.Count(querySeg)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list guilds error")))
		return
	}

	commonBudgetSources, err := model.QueryRows[model.CommonBudgetSource](querySeg, page)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list guilds error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&api.ListReplyData{
		Page:  page.Page,
		Size:  page.Size,
		Total: total,
		Rows:  commonBudgetSources,
	}))
}
