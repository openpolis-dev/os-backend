package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/task_manager"
	"gorm.io/gorm"
)

var err error

type autoXferTaskResponse struct {
	ID        int   `json:"id"`
	CreateTs  int64 `json:"create_ts"`
	UpdateTs  int64 `json:"update_ts"`
	ExecuteTs int64 `json:"execute_ts"`

	State string `json:"state"`

	TransactionItems []*task_manager.AutoTransferScrItem `json:"transaction_items"`

	IsFinished bool `json:"is_finished"`

	TransactionHash string `json:"transaction_hash"`
}

type AutoXferTaskListQueryParams struct {
	Page      int    `form:"page"`
	Size      int    `form:"size"`
	SortField string `form:"sort_field"`
	SortOrder string `form:"sort_order"`

	State string `form:"state"`
}

func AutoXferTaskList(ctx *gin.Context) {
	queryParams := AutoXferTaskListQueryParams{}
	if err = ctx.Bind(&queryParams); err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("query params error: %+v", err),
		})
		return
	}

	pageParams := api.ParseAndConvertPageParam(ctx)

	db := api.ForContextOnlyDB(ctx)
	var tasks []model.CronJob
	query := db.Model(&model.CronJob{}).Where("handler_name = ?", internal.TaskAutoTransferSCR)

	if pageParams.Page > 0 && pageParams.Size > 0 {
		query = query.Offset((pageParams.Page - 1) * pageParams.Size).Limit(pageParams.Size)
	}

	if pageParams.SortField != nil && pageParams.Order != nil {
		query = query.Order(fmt.Sprintf("%s %s", *pageParams.SortField, *pageParams.Order))
	}

	if queryParams.State != "" {
		query = query.Where("state = ?", queryParams.State)
	}

	err := query.Find(&tasks).Error
	if err != nil {
		log.Error().Msgf("get auto transfer SCR tasks error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get auto transfer SCR tasks error")))
		return
	}

	response := lo.Map(tasks, func(task model.CronJob, _ int) *autoXferTaskResponse {
		return generateAutoXferTaskResponse(task)
	})

	ctx.JSON(http.StatusOK, api.Success(response))
}

func AutoXferTaskDetail(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)
	task, err := getCronJobRecordFromStrId(db, ctx.Param("id"))
	if err != nil {
		log.Error().Msgf("get auto transfer SCR task error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get auto transfer SCR task error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(generateAutoXferTaskResponse(task)))
}

// CancelAutoXferTask cancels a auto transfer SCR task
func CancelAutoXferTask(ctx *gin.Context) {
	user, enforcer, db, _ := api.ForContext(ctx)

	ok, err := enforcer.HasRoleForUser(common.FormatUserWallet(user.Wallet), internal.RoleHall)
	if err != nil {
		log.Error().Msgf("check permission error %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error")))
		return
	}

	if !ok {
		log.Warn().Msgf("permission deny for user %s", common.FormatUserWallet(user.Wallet))
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	task, err := getCronJobRecordFromStrId(db, ctx.Param("id"))
	if err != nil {
		log.Error().Msgf("get auto transfer SCR task error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get auto transfer SCR task error")))
		return
	}

	err = db.Model(&model.CronJob{}).Where("id = ?", task.ID).Update("state", model.CronJobStateTerminated).Error
	if err != nil {
		log.Error().Msgf("cancel auto transfer SCR task error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("cancel auto transfer SCR task error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func generateAutoXferTaskResponse(task model.CronJob) *autoXferTaskResponse {
	var taskParams *task_manager.AutoTransferScrParam
	err = json.Unmarshal([]byte(task.JobParams), &taskParams)
	if err != nil {
		log.Error().Msgf("parse auto transfer SCR params error: %+v", err)
		return nil
	}

	resp := autoXferTaskResponse{
		ID:       task.ID,
		CreateTs: task.CreateTs,
		UpdateTs: task.UpdateTs,

		State: task.StateName(),

		TransactionItems: taskParams.Items,

		IsFinished: task.State == model.CronJobStateDone,
	}

	var execResult task_manager.AutoTransferScrTaskResult
	if task.LastExecResult != "" {
		err = json.Unmarshal([]byte(task.LastExecResult), &execResult)
		if err != nil {
			log.Error().Msgf("parse auto transfer SCR task result error: %+v", err)
		}
		resp.TransactionHash = execResult.Data.TxHash
	}

	return &resp
}

func getCronJobRecordFromStrId(db *gorm.DB, taskId string) (model.CronJob, error) {
	var task model.CronJob
	err = db.Model(&model.CronJob{}).Where("id = ?", taskId).Find(&task).Error
	if err != nil {
		return model.CronJob{}, err
	}

	return task, nil
}
