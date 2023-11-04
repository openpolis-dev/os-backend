package publicdata

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/sdk"
)

// NotionDatabase returns the list of notion database
// @Summary NotionDatabase returns the list of notion database
// @Tags PublicData
// @Accept json
// @Produce json
// @Param page query int false "page"
// @Param size query int false "size"
// @Router /public_data/notion/database/{id} [get]
// @Success 200 {object} api.Reply{data=api.ListReplyData}
func NotionDatabase(ctx *gin.Context) {
	databaseId := ctx.Param("id")

	_, cfg := api.ForContextDBAndConfig(ctx)

	body, err := ctx.GetRawData()
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.ServerError(err))
		return
	}

	data, err := sdk.NotionDatabase(databaseId, cfg.PublicData.Notion.APIToken, body)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	var databaseData sdk.NotionDatabaseData
	err = json.Unmarshal(data, &databaseData)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	page := api.ParseAndConvertPageParam(ctx)
	// [0, 10)
	start := (page.Page - 1) * page.Size
	end := page.Page*page.Size - 1

	total := len(databaseData.Result)
	if total < start {
		ctx.JSON(http.StatusOK, api.Success(&api.ListReplyData{
			Page:  page.Page,
			Size:  page.Size,
			Total: int64(total),
			Rows:  []any{},
		}))
		return
	}
	if total < end {
		end = total - 1
	}

	ctx.JSON(http.StatusOK, api.Success(&api.ListReplyData{
		Page:  page.Page,
		Size:  page.Size,
		Total: int64(total),
		Rows:  databaseData.Result[start:end],
	}))
}

// NotionPage returns the detail of a notion page
// @Summary NotionPage returns the detail of a notion page
// @Tags PublicData
// @Accept json
// @Produce json
// @Param id path string true "id"
// @Router /public_data/notion/page/{id} [get]
// @Success 200 {object} api.Reply
func NotionPage(ctx *gin.Context) {
	pageId := ctx.Param("id")

	_, cfg := api.ForContextDBAndConfig(ctx)

	data, err := sdk.NotionPage(pageId, cfg.PublicData.Notion.APIToken)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	var pageData map[string]any
	err = json.Unmarshal(data, &pageData)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(pageData))
}

// NotionUser returns the detail of a notion user
// @Summary NotionUser returns the detail of a notion user
// @Tags PublicData
// @Accept json
// @Produce json
// @Param id path string true "id"
// @Router /public_data/notion/user/{id} [get]
// @Success 200 {object} api.Reply
func NotionUser(ctx *gin.Context) {
	userId := ctx.Param("id")

	_, cfg := api.ForContextDBAndConfig(ctx)

	data, err := sdk.NotionUser(userId, cfg.PublicData.Notion.APIToken)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	var pageData map[string]any
	err = json.Unmarshal(data, &pageData)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(pageData))
}
