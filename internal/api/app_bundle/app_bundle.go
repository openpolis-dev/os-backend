package app_bundle

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
)

type AppBundleRecord struct {
	SeasonName string               `json:"season_name"`
	Records    []*model.Application `json:"records"`
	Entity     struct {
		Id   uint   `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"entity"`
	Submitter  string    `json:"submitter"`
	SubmitDate time.Time `json:"submit_date"`
	Reviewer   string    `json:"reviewer"`
	Comment    string    `json:"comment"`
	State      string    `json:"state"`
	Assets     []struct {
		Name   string `json:"name"`
		Amount string `json:"amount"`
	} `json:"assets"`
}

func BuildResponseFromDatabaseSearchResult() {

}

// ListAppBundle returns application bundles with passed in query types
//
//	@Summary		List all application bundles match the query params
//	@Router			/app_bundles [get]
//	@Tags			app_bundle
//	@Param			status		query		string	false	"status of application bundle"	Enum(open approved rejected processing completed)
//	@Param			page		query		string	false	"which page"
//	@Param			size		query		string	false	"size of each page"
//	@Param			sort_field	query		string	false	"sort by which field"
//	@Param			sort_order	query		string	false	"order of sort"	Enum(asc desc)
//
//	@Success		200			{object}	AppBundleRecord
func ListAppBundle(ctx *gin.Context) {
	var err error
	db := api.ForContextOnlyDB(ctx)

	queryParams := model.ListAppBundleQueryParams{}

	rcds, total, err := model.QueryAppBundleRecords(db, &queryParams)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("query result error: %+v", err),
		})
		return
	}

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  queryParams.Page,
		Size:  queryParams.Size,
		Total: total,
		Rows:  rcds,
	}))
}

func CreateAppBundle(ctx *gin.Context) {}
