package publicdata

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/sdk"
)

// BountyList returns the list of bounties
//	@Summary	BountyList returns the list of bounties
//	@Tags		PublicData
//	@Accept		json
//	@Produce	json
//	@Param		page	query	int	false	"page"
//	@Param		size	query	int	false	"size"
//	@Router		/public_data/bounty/list [get]
//	@Success	200	{object}	api.Reply{data=api.ListReplyData}
func BountyList(ctx *gin.Context) {
	_, cfg := api.ForContextDBAndConfig(ctx)

	//curl -X POST https://api.notion.com/v1/databases/73d83a0a258d4ac5afa57a997114755a/query -H 'Authorization: Bearer secret_gnVFq5NWrDHY481DoMPwaC' -H "Content-Type: application/json" -H "Notion-Version: 2022-06-28" --data '{}'}
	body, err := ctx.GetRawData()
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.ServerError(err))
		return
	}

	data, err := sdk.NotionDatabase(cfg.PublicData.Notion.BountyDatabaseID, cfg.PublicData.Notion.APIToken, body)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	var databaseData sdk.DatabaseData
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

// BountyDetail returns the detail of a bounty
//	@Summary	BountyDetail returns the detail of a bounty
//	@Tags		PublicData
//	@Accept		json
//	@Produce	json
//	@Param		id	path	string	true	"id"
//	@Router		/public_data/bounty/detail/{id} [get]
//	@Success	200	{object}	api.Reply
func BountyDetail(ctx *gin.Context) {
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
