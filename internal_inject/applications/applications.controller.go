package applications_inject

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

type ApplicationsController struct {
	// inject

	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	ApplicationsService *ApplicationsService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var applications ApplicationsController

	err := inject.Populate(&applications, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var applicationsGroup *gin.RouterGroup
	var applicationsAuthGroup *gin.RouterGroup

	if fatherGroup != nil {
		applicationsGroup = fatherGroup.Group("/applications")
		applicationsAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/applications")
	} else {
		applicationsGroup = applications.Gin.Group("/applications")
		applicationsAuthGroup = applications.Gin.Group("/", middleware.AuthRequired).Group("/applications")
	}

	// no auth
	applicationsGroup.GET("/", applications.List)
	applicationsGroup.GET("/:id", applications.Detail)
	if fatherGroup != nil {
		fatherGroup.GET("/apps_applicants", applications.ListApplicants)
		fatherGroup.GET("/download_applications", applications.Download)
	} else {
		applications.Gin.GET("/apps_applicants", applications.ListApplicants)
		applications.Gin.GET("/download_applications", applications.Download)
	}
	applicationsGroup.GET("/assets/statistics", applications.AssetStatistics)

	// auth
	applicationsAuthGroup.POST("/", applications.Create)

	if fatherGroup != nil {
		// batch application routers
		fatherGroup.Group("/", middleware.AuthRequired).POST("/apps_approve", applications.BatchApprove)
		fatherGroup.Group("/", middleware.AuthRequired).POST("/apps_reject", applications.BatchReject)
		fatherGroup.Group("/", middleware.AuthRequired).POST("/apps_process", applications.BatchProcess)
		fatherGroup.Group("/", middleware.AuthRequired).POST("/apps_complete", applications.BatchComplete)

		// Auto transfer SCR application routers
		fatherGroup.Group("/", middleware.AuthRequired).GET("/scr_tasks/", applications.AutoXferTaskList)
		fatherGroup.Group("/", middleware.AuthRequired).GET("/scr_tasks/:id", applications.AutoXferTaskDetail)
		fatherGroup.Group("/", middleware.AuthRequired).POST("/scr_tasks/:id/cancel", applications.CancelAutoXferTask)
	} else {
		// batch application routers
		applications.Gin.Group("/", middleware.AuthRequired).POST("/apps_approve", applications.BatchApprove)
		applications.Gin.Group("/", middleware.AuthRequired).POST("/apps_reject", applications.BatchReject)
		applications.Gin.Group("/", middleware.AuthRequired).POST("/apps_process", applications.BatchProcess)
		applications.Gin.Group("/", middleware.AuthRequired).POST("/apps_complete", applications.BatchComplete)

		// Auto transfer SCR application routers
		applications.Gin.Group("/", middleware.AuthRequired).GET("/scr_tasks/", applications.AutoXferTaskList)
		applications.Gin.Group("/", middleware.AuthRequired).GET("/scr_tasks/:id", applications.AutoXferTaskDetail)
		applications.Gin.Group("/", middleware.AuthRequired).POST("/scr_tasks/:id/cancel", applications.CancelAutoXferTask)
	}
}

func (c *ApplicationsController) List(ctx *gin.Context) {
	queryParams := model.ListApplicationQueryParams{}
	if err := ctx.Bind(&queryParams); err != nil {
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("query params error: %+v", err)))
		return
	}

	rcds, total, err := model.GenerateFrontendApplicationRecords(c.Db, &queryParams, true)
	if err != nil {
		log.Error().Msgf("list application API error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query result error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  queryParams.Page,
		Size:  queryParams.Size,
		Total: total,
		Rows:  rcds,
	}))
}

func (c *ApplicationsController) Detail(ctx *gin.Context) {
	application := &model.Application{}
	c.ApplicationsService.GetRecordOrReturnNotFound(ctx, application)
	ctx.JSON(http.StatusOK, application)
}

func (c *ApplicationsController) ListApplicants(ctx *gin.Context) {
	var err error

	var rslt []ApplicantListResponse

	err = c.Db.Model(&model.Application{}).
		Distinct("wallet").
		Joins("inner join users on users.wallet = applications.applicant").
		Select("applications.applicant, users.name").
		Find(&rslt).Error
	if err != nil {
		log.Error().Msgf("query applications error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query applications error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(rslt))

}

func (c *ApplicationsController) Download(ctx *gin.Context) {
	var err error

	queryParams := model.ListApplicationQueryParams{}
	if err := ctx.Bind(&queryParams); err != nil {
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("query params error: %+v", err)))
		return
	}

	rcds, _, err := model.GenerateFrontendApplicationRecords(c.Db, &queryParams, false)
	if err != nil {
		log.Error().Msgf("list application API error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query result error detail:"+err.Error())))
		return
	}

	// Generated header with passed in lang
	lang := api.GetLangFromQuery(ctx, "en")
	headerStr := internal.ApplicationDownloadHeader[lang]
	if header, found := internal.ApplicationDownloadHeader[lang]; found {
		headerStr = header
	}
	csvHeaderList := strings.Split(headerStr, ",")

	// Get file format will be generated
	fileFormat := strings.ToLower(api.GetQueryParamsOrDefaultValue(ctx, "format", "xlsx"))

	if fileFormat == "csv" {
		buf := new(bytes.Buffer)
		w := csv.NewWriter(buf)
		err = w.Write(csvHeaderList)
		if err != nil {
			log.Error().Msgf("write csv header error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("write csv header error detail:"+err.Error())))
			return
		}

		for _, r := range rcds {
			if r == nil {
				continue
			}
			err = w.Write([]string{
				r.TargetUserWallet,
				fmt.Sprintf("%s %s", r.Amount, r.AssetName),
				r.SeasonName,
				r.DetailedType,
				r.BudgetSource,
				r.ApplicantWallet,
				r.Status,
			})
			if err != nil {
				log.Error().Msgf("write csv row error: %+v", err)
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("write csv row error detail:"+err.Error())))
				return
			}
		}
		w.Flush()

		r := bytes.NewReader(buf.Bytes())

		contentLength := buf.Len()

		extraHeaders := map[string]string{
			"Content-Disposition": `attachment; filename="applications-list.csv"`,
		}

		ctx.DataFromReader(http.StatusOK, int64(contentLength), "encoding/csv", r, extraHeaders)
	} else if fileFormat == "xlsx" {
		fileName := "applications-list.xlsx"

		// create excel stream writer
		f := excelize.NewFile()
		streamWriter, err := f.NewStreamWriter("Sheet1")
		if err != nil {
			log.Error().Msgf("create excel stream writer error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create excel stream writer error detail:"+err.Error())))
			return
		}

		// write first row
		cell, _ := excelize.CoordinatesToCellName(1, 1)
		title := lo.Map(csvHeaderList, func(item string, _ int) any { return item })
		if err := streamWriter.SetRow(cell, title); err != nil {
			log.Error().Msgf("write excel header error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("write excel header error detail:"+err.Error())))
			return
		}

		rowId := 2
		for _, row := range rcds {
			cell, _ := excelize.CoordinatesToCellName(1, rowId)
			err = streamWriter.SetRow(cell, []any{
				row.TargetUserWallet,
				fmt.Sprintf("%s %s", row.Amount, row.AssetName),
				row.SeasonName,
				row.DetailedType,
				row.BudgetSource,
				row.ApplicantWallet,
				row.Status,
			})
			if err != nil {
				log.Error().Msgf("write excel row[%d] failed: %s", rowId, err)
			}
			rowId += 1
		}

		ctx.Header("Content-Disposition", `attachment; filename="`+fileName+`"`)

		// flush writer
		if err = streamWriter.Flush(); err != nil {
			log.Error().Msgf("flush writer [%s] failed: %s", fileName, err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("flush excel writer error detail:"+err.Error())))
			return
		}

		// write to response
		err = f.Write(ctx.Writer)
		if err != nil {
			log.Error().Msgf("write excel to response error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("write excel to response error detail:"+err.Error())))
		}
	} else if fileFormat == "json" {
		tmpFile, _ := os.CreateTemp(os.TempDir(), "application-list-*.json")
		defer os.Remove(tmpFile.Name())

		fileBaseName := filepath.Base(tmpFile.Name())

		jsonBytes, err := json.Marshal(rcds)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("marshal json error detail:"+err.Error())))
			return
		}

		err = os.WriteFile(tmpFile.Name(), jsonBytes, 0777)
		if err != nil {
			log.Error().Msgf("write json error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("write json error detail:"+err.Error())))
			return
		}

		ctx.FileAttachment(tmpFile.Name(), fileBaseName)
		ctx.Writer.Header().Set("Content-Disposition", `attachment; filename="`+fileBaseName+`"`)
	} else {
		_, _ = ctx.Writer.Write([]byte(""))
	}
}

func (c *ApplicationsController) Create(ctx *gin.Context) {
	httpCode, reply := c.ApplicationsService.Create(ctx)

	ctx.JSON(httpCode, reply)
}

func (c *ApplicationsController) BatchApprove(ctx *gin.Context) {
	var applications []*model.Application
	err := c.ApplicationsService.GetBatchApplicationsOrReturnError(ctx, &applications)
	if err != nil {
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("parse request data error detail:"+err.Error())))
		return
	}

	user, enforcer, _, _ := api.ForContext(ctx)

	//  check permission: `(0x..., proj_and_guild, audit_app)`
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), internal.ObjProjAndGuild, internal.ActAuditApplication)
	if err != nil {
		log.Error().Msgf("check permission error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error detail:"+err.Error())))
		return
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.ObjProjAndGuild, internal.ActCreateApplication)
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	// FIXME: pass application ids instead of reference to applications
	push := api.ForContextOnlyPush(ctx)
	err = model.BatchAuditApplication(c.Db, common.FormatUserWallet(user.Wallet), &applications, model.AuditActionApprove, "", enforcer, push)
	if err != nil {
		log.Error().Msgf("approve applications error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("approve applications error detail:"+err.Error())))
		return
	}

	err = api.CreateAutoTransferScrTask(c.Db, applications, 0)
	if err != nil {
		log.Error().Msgf("create auto transfer SCR task error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create auto transfer SCR task error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, "")
}

func (c *ApplicationsController) BatchReject(ctx *gin.Context) {
	var applications []*model.Application
	err := c.ApplicationsService.GetBatchApplicationsOrReturnError(ctx, &applications)
	if err != nil {
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("parse request data error detail:"+err.Error())))
		return
	}

	user, enforcer, db, _ := api.ForContext(ctx)

	//  check permission: `(0x..., proj_and_guild, audit_app)`
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), internal.ObjProjAndGuild, internal.ActAuditApplication)
	if err != nil {
		log.Error().Msgf("check permission error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error detail:"+err.Error())))
		return
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.ObjProjAndGuild, internal.ActCreateApplication)
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	push := api.ForContextOnlyPush(ctx)
	err = model.BatchAuditApplication(db, common.FormatUserWallet(user.Wallet), &applications, model.AuditActionReject, "", enforcer, push)
	if err != nil {
		log.Error().Msgf("reject applications error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.ServerError(errors.New("reject applications error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, "")
}

func (c *ApplicationsController) BatchProcess(ctx *gin.Context) {
	user, enforcer, _, _ := api.ForContext(ctx)

	//  check permission: `(0x..., proj_and_guild, audit_app)`
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), internal.ObjProjAndGuild, internal.ActAuditApplication)
	if err != nil {
		log.Error().Msgf("check permission error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error detail:"+err.Error())))
		return
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.ObjProjAndGuild, internal.ActCreateApplication)
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	var processingRecordCount int64
	c.Db.Model(&model.Application{}).Where("state = ?", model.ApplicationStateProcessing).Count(&processingRecordCount)

	if processingRecordCount > 0 {
		sdk.LogUserSideError(ctx, errors.New("applications in processing state should be processed before exporting new list"))
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("applications in processing state should be processed before exporting new list")))
		return
	}

	var applications []*model.Application
	err = c.Db.Model(&model.Application{}).Where("state = ?", model.ApplicationStateApproved).Find(&applications).Error
	if err != nil {
		log.Error().Msgf("query approved applications error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query approved applications error detail:"+err.Error())))
		return
	}

	push := api.ForContextOnlyPush(ctx)

	err = model.BatchAuditApplication(c.Db, common.FormatUserWallet(user.Wallet), &applications, model.AuditActionProcess, "", enforcer, push)
	if err != nil {
		log.Error().Msgf("process applications error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("process applications error detail:"+err.Error())))
		return
	}

	// TODO: Merge this logic with `BatchComplete`
	appBundleIds := make(map[uint]bool)
	for _, app := range applications {
		appBundleIds[app.BundleId] = true
	}
	err = c.Db.Model(&model.AppBundle{}).Where("id IN ?", lo.Keys(appBundleIds)).Update("state", model.ApplicationStateProcessing).Error
	if err != nil {
		log.Error().Msgf("process applications error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("process applications error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(applications))
}

func (c *ApplicationsController) BatchComplete(ctx *gin.Context) {
	user, enforcer, _, _ := api.ForContext(ctx)

	//  check permission: `(0x..., proj_and_guild, audit_app)`
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), internal.ObjProjAndGuild, internal.ActAuditApplication)
	if err != nil {
		log.Error().Msgf("check permission error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error detail:"+err.Error())))
		return
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.ObjProjAndGuild, internal.ActCreateApplication)
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	var applications []*model.Application
	c.Db.Model(&model.Application{}).Where("state = ?", model.ApplicationStateProcessing).Find(&applications)

	reqBody := AuditRequestBody{}
	err = ctx.Bind(&reqBody)
	if err != nil {
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("parse request data error detail:"+err.Error())))
		return
	}

	// FIXME: pass application ids instead of reference to applications
	push := api.ForContextOnlyPush(ctx)
	err = model.BatchAuditApplication(c.Db, common.FormatUserWallet(user.Wallet), &applications, model.AuditActionComplete, reqBody.Message, enforcer, push)
	if err != nil {
		log.Error().Msgf("complete applications error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.ServerError(errors.New("complete applications error detail:"+err.Error())))
		return
	}

	appBundleIds := make(map[uint]bool)
	for _, app := range applications {
		appBundleIds[app.BundleId] = true
	}
	err = c.Db.Model(&model.AppBundle{}).Where("id IN ?", lo.Keys(appBundleIds)).Update("state", model.ApplicationStateCompleted).Error
	if err != nil {
		log.Error().Msgf("complete applications error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.ServerError(errors.New("complete applications error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, "")
}

func (c *ApplicationsController) AutoXferTaskList(ctx *gin.Context) {
	queryParams := AutoXferTaskListQueryParams{}
	if err := ctx.Bind(&queryParams); err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("query params error: %+v", err),
		})
		return
	}

	pageParams := api.ParseAndConvertPageParam(ctx)

	var tasks []model.CronJob
	query := c.Db.Model(&model.CronJob{}).Where("handler_name = ?", internal.TaskAutoTransferSCR)

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
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get auto transfer SCR tasks error detail:"+err.Error())))
		return
	}

	response := lo.Map(tasks, func(task model.CronJob, _ int) *AutoXferTaskResponse {
		return c.ApplicationsService.GenerateAutoXferTaskResponse(task)
	})

	ctx.JSON(http.StatusOK, api.Success(response))
}

func (c *ApplicationsController) AutoXferTaskDetail(ctx *gin.Context) {
	task, err := c.ApplicationsService.GetCronJobRecordFromStrId(ctx.Param("id"))
	if err != nil {
		log.Error().Msgf("get auto transfer SCR task error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get auto transfer SCR task error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(c.ApplicationsService.GenerateAutoXferTaskResponse(task)))

}

func (c *ApplicationsController) CancelAutoXferTask(ctx *gin.Context) {
	user, enforcer, _, _ := api.ForContext(ctx)

	ok, err := enforcer.HasRoleForUser(common.FormatUserWallet(user.Wallet), internal.RoleHall)
	if err != nil {
		log.Error().Msgf("check permission error %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error detail:"+err.Error())))
		return
	}

	if !ok {
		log.Warn().Msgf("permission deny for user %s", common.FormatUserWallet(user.Wallet))
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	task, err := c.ApplicationsService.GetCronJobRecordFromStrId(ctx.Param("id"))
	if err != nil {
		log.Error().Msgf("get auto transfer SCR task error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get auto transfer SCR task error detail:"+err.Error())))
		return
	}

	err = c.Db.Model(&model.CronJob{}).Where("id = ?", task.ID).Update("state", model.CronJobStateTerminated).Error
	if err != nil {
		log.Error().Msgf("cancel auto transfer SCR task error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("cancel auto transfer SCR task error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func (c *ApplicationsController) AssetStatistics(ctx *gin.Context) {
	var waitForGrantUsd float64
	var waitForGrantScr float64

	var grantedUsd float64
	var grantedScr float64

	var checkingUsd float64
	var checkingScr float64

	currentSeason, err := model.GetCurrentSeason(c.Db)
	if err != nil {
		log.Error().Msgf("fetch current season error: %v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("fetch current season error")))
		return
	}

	waitForGrantUsd, err = c.ApplicationsService.SumAssetAmount(ctx, string(model.ApplicationStateOpen), "USD", int(currentSeason.ID))
	if err != nil {
		log.Error().Msgf("get sum wait grant usd amount error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get sum wait grant usd amount error detail:"+err.Error())))
		return
	}

	waitForGrantScr, err = c.ApplicationsService.SumAssetAmount(ctx, string(model.ApplicationStateOpen), "SCR", int(currentSeason.ID))
	if err != nil {
		log.Error().Msgf("get sum wait grant scr amount error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get sum wait grant scr amount error detail:"+err.Error())))
		return
	}

	grantedUsd, err = c.ApplicationsService.SumAssetAmount(ctx, string(model.ApplicationStateCompleted), "USD", int(currentSeason.ID))
	if err != nil {
		log.Error().Msgf("get sum granted usd amount error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get sum granted usd amount error detail:"+err.Error())))
		return
	}

	grantedScr, err = c.ApplicationsService.SumAssetAmount(ctx, string(model.ApplicationStateCompleted), "SCR", int(currentSeason.ID))
	if err != nil {
		log.Error().Msgf("get sum granted scr amount error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get sum granted scr amount error detail:"+err.Error())))
		return
	}

	checkingUsd, err = c.ApplicationsService.SumAssetAmount(ctx, string(model.ApplicationStateApproved), "USD", int(currentSeason.ID))
	if err != nil {
		log.Error().Msgf("get sum checking usd amount error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get sum checking usd amount error detail:"+err.Error())))
		return
	}

	checkingScr, err = c.ApplicationsService.SumAssetAmount(ctx, string(model.ApplicationStateApproved), "SCR", int(currentSeason.ID))
	if err != nil {
		log.Error().Msgf("get sum checking scr amount error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get sum checking scr amount error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&ApplicationAssetStatistic{
		WaitForGrantUsd: waitForGrantUsd,
		WaitForGrantScr: waitForGrantScr,
		GrantedUsd:      grantedUsd,
		GrantedScr:      grantedScr,
		CheckingUsd:     checkingUsd,
		CheckingScr:     checkingScr,
	}))
}
