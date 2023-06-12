package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/model"
)

var err error

func ApplicationList(ctx *gin.Context) {
	_, db, _ := ForContext(ctx)

	// TODO: enable load this field from query params
	orderBy := "updated_at desc"

	pageSize := DefaultPageSize
	passedInPageSize, found := ctx.GetQuery("size")
	if found {
		pageSize, err = strconv.Atoi(passedInPageSize)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, Reply{
				Code: -1,
				Msg:  "Error size",
			})
		}
	}

	queryOffset := 0
	passedInPageCnt, found := ctx.GetQuery("page")
	if found {
		pageCnt, err := strconv.Atoi(passedInPageCnt)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, Reply{
				Code: -1,
				Msg:  "Error page",
			})
		} else {
			queryOffset = pageCnt * pageSize
		}
	}

	var rcds []model.Application
	db.Offset(queryOffset).Limit(pageSize).Order(orderBy).Find(&rcds)
	ctx.JSON(http.StatusOK, Success(rcds))
}

// ApplicationCreate handles creating application with passed in data
// An audit log record will be created with application at same time with action open
func ApplicationCreate(ctx *gin.Context) {

}

func ApplicationDetail(ctx *gin.Context) {}

func ApplicationBatchApprove(ctx *gin.Context)  {}
func ApplicationBatchReject(ctx *gin.Context)   {}
func ApplicationBatchComplete(ctx *gin.Context) {}

func ApplicationApprove(ctx *gin.Context)  {}
func ApplicationReject(ctx *gin.Context)   {}
func ApplicationComplete(ctx *gin.Context) {}
