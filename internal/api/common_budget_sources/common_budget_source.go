package common_budget_sources

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/xiaosongfu/gormfind"
)

func List(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)

	page := api.ParseAndConvertPageParam(ctx)

	keywords := ctx.Query("keywords")
	wallet := ctx.Query("wallet")

	querySeg := db.Model(&model.CommonBudgetSource{})
	if keywords != "" {
		querySeg.Where(fmt.Sprintf("name ILIKE '%%%s%%'", keywords))
	}
	if wallet != "" {
		w := fmt.Sprintf("%%\"%s\"%%", wallet) // value is: `%0x123%`
		querySeg.Where(fmt.Sprintf("sponsors::text ILIKE '%%%s%%'", w))
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
