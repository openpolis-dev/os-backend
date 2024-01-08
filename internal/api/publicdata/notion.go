package publicdata

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/sdk"
)

// NotionDatabase returns the list of notion database
//
//	@summary	NotionDatabase returns the list of notion database
//	@tags		PublicData
//	@accept		json
//	@produce	json
//	@param		page	query	int	false	"page"
//	@param		size	query	int	false	"size"
//	@router		/public_data/notion/database/{id} [get]
//	@success	200	{object}	api.Reply{data=api.ListReplyData}
func NotionDatabase(ctx *gin.Context) {
	databaseId := ctx.Param("id")

	_, cfg := api.ForContextDBAndConfig(ctx)

	body, err := ctx.GetRawData()
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.ServerError(errors.New("get raw data error")))
		return
	}

	data, err := sdk.NotionDatabase(databaseId, cfg.PublicData.Notion.APIToken, body)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("notion database error")))
		return
	}

	var databaseData sdk.NotionDatabaseData
	err = json.Unmarshal(data, &databaseData)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("notion database error")))
		return
	}

	page := api.ParseAndConvertPageParam(ctx)
	// [0, 10)
	start := (page.Page - 1) * page.Size
	end := page.Page * page.Size

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
		end = total
	}

	ctx.JSON(http.StatusOK, api.Success(&api.ListReplyData{
		Page:  page.Page,
		Size:  page.Size,
		Total: int64(total),
		Rows:  databaseData.Result[start:end],
	}))
}

// NotionPage returns the detail of a notion page
//
//	@summary	NotionPage returns the detail of a notion page
//	@tags		PublicData
//	@accept		json
//	@produce	json
//	@param		id	path	string	true	"id"
//	@router		/public_data/notion/page/{id} [get]
//	@success	200	{object}	api.Reply
func NotionPage(ctx *gin.Context) {
	pageId := ctx.Param("id")

	_, cfg := api.ForContextDBAndConfig(ctx)

	data, err := sdk.NotionPage(pageId, cfg.PublicData.Notion.APIToken)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("notion page error")))
		return
	}

	var pageData map[string]any
	err = json.Unmarshal(data, &pageData)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("notion page error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(pageData))
}

// NotionUser returns the detail of a notion user
//
//	@summary	NotionUser returns the detail of a notion user
//	@tags		PublicData
//	@accept		json
//	@produce	json
//	@param		id	path	string	true	"id"
//	@router		/public_data/notion/user/{id} [get]
//	@success	200	{object}	api.Reply
func NotionUser(ctx *gin.Context) {
	userId := ctx.Param("id")

	_, cfg := api.ForContextDBAndConfig(ctx)

	data, err := sdk.NotionUser(userId, cfg.PublicData.Notion.APIToken)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("notion user error")))
		return
	}

	var pageData map[string]any
	err = json.Unmarshal(data, &pageData)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("notion user error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(pageData))
}
