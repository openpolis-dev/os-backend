package application

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
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

type AuditRequestBody struct {
	Message string `json:"message"`
}

type ApplicantListResponse struct {
	Applicant string
	Name      string
}

// ListApplicants list all applicants existing in applications table for filter
//
//	@summary	List all applicants existing in applications table for filter
//	@router		/apps_applicants [get]
//	@success	200	{object}	ApplicantListResponse
func ListApplicants(ctx *gin.Context) {
	var err error
	db := api.ForContextOnlyDB(ctx)

	var rslt []ApplicantListResponse

	err = db.Model(&model.Application{}).
		Distinct("wallet").
		Joins("inner join users on users.wallet = applications.applicant").
		Select("applications.applicant, users.name").
		Find(&rslt).Error
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(rslt))
}

// List lists all applications based on query params and return in JSON format
//
//	@summary	lists all applications based on query params and return in JSON format
//	@router		/applications [get]
//	@success	200	{object}	api.Reply{data=api.ListReplyData{rows=model.FrontendApplicationRecord}}
func List(ctx *gin.Context) {
	var err error
	db := api.ForContextOnlyDB(ctx)

	queryParams := model.ListApplicationQueryParams{}
	if err := ctx.Bind(&queryParams); err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("query params error: %+v", err),
		})
		return
	}

	rcds, total, err := model.GenerateFrontendApplicationRecords(db, &queryParams, true)
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

// Create handles creating one or more application records with passed in data, the passed in data must be an array
// An audit log record will be created with application at same time with action open
//
//	@summary	create single application, for now only CLOSE_PROJECT type is allowed
//	@router		/applications [post]
//	@param		JsonBody	body		[]model.NewApplicationRequest	true	"new application request"
//	@success	200			{string}	nil
func Create(ctx *gin.Context) {
	var newApplicationReqs []model.NewApplicationRequest
	if err := ctx.BindJSON(&newApplicationReqs); err != nil {
		if err != nil {
			ctx.JSON(http.StatusBadRequest, api.Reply{
				Code: -1,
				Msg:  fmt.Sprintf("passed in data error: %+v", err),
			})
		}
		return
	}

	user, enforcer, db, _ := api.ForContext(ctx)

	err := db.Transaction(func(tx *gorm.DB) error {
		for _, req := range newApplicationReqs {
			// TODO: Verify user wallet and project/guild existing

			// Parse application type
			appType, err := model.ParseApplicationType(req.Type)
			if err != nil {
				return fmt.Errorf("unknown application entity %s", req.Entity)
			}

			if appType == model.ApplicationNewReward {
				return fmt.Errorf("NEW_REWARD application should be submitted via app_bundle")
			}

			if req.Entity != "guild" && req.Entity != "project" {
				return fmt.Errorf("unknown application entity %s", req.Entity)
			}

			//  check permission: `(0x..., proj_1, create_app)` (0x..., guild_1, create_app)
			obj := lo.
				If(req.Entity == "project", fmt.Sprintf("%s%d", api.ObjProjPrefix, req.EntityId)).
				ElseIf(req.Entity == "guild", fmt.Sprintf("%s%d", api.ObjGuildPrefix, req.EntityId)).
				Else("")
			ok, err := enforcer.Enforce(user.Wallet, obj, api.ActCreateApplication)
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
				return err
			}
			if !ok {
				ctx.JSON(http.StatusForbidden, api.Forbidden())
				return err
			}

			seasonRecord, err := model.GetCurrentSeason(db)
			if err != nil {
				return err
			}

			appBundle := model.AppBundle{
				Applicant:    user.Wallet,
				EntityType:   req.Entity,
				EntityId:     req.EntityId,
				SeasonId:     seasonRecord.ID,
				State:        model.ApplicationStateOpen,
				CreatedAt:    time.Now().In(internal.ProjectTimezone),
				UpdatedAt:    time.Now().In(internal.ProjectTimezone),
				CreateTs:     model.GetCurrentUtcEpochSecond(),
				UpdateTs:     model.GetCurrentUtcEpochSecond(),
				ShadowRecord: true,
				Type:         "CLOSE_PROJECT",
			}
			err = db.Save(&appBundle).Error
			if err != nil {
				return err
			}

			app := &model.Application{
				Type:         appType,
				Applicant:    user.Wallet,
				State:        model.ApplicationStateOpen,
				EntityType:   req.Entity,
				EntityId:     req.EntityId,
				SeasonId:     seasonRecord.ID,
				DetailedType: req.DetailedType,
				Comment:      req.Comment,
				CreatedAt:    time.Now().In(internal.ProjectTimezone),
				UpdatedAt:    time.Now().In(internal.ProjectTimezone),
				CreateTs:     model.GetCurrentUtcEpochSecond(),
				UpdateTs:     model.GetCurrentUtcEpochSecond(),
				BundleId:     appBundle.ID,
			}

			err = model.NewApplicationRecord(db, app)
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("creation application records error, %+v", err),
		})
		return
	}

	ctx.JSON(http.StatusCreated, api.Success(nil))
}

// Download downloads all applications based on query params and sends Excel file for downloading
//
//	@summary	downloads all applications based on query params and sends Excel file for downloading
//	@router		/download_applications [get]
//	@success	200	{object}	api.Reply{data=api.ListReplyData{rows=model.FrontendApplicationRecord}}
func Download(ctx *gin.Context) {
	var err error
	db := api.ForContextOnlyDB(ctx)

	queryParams := model.ListApplicationQueryParams{}
	if err := ctx.Bind(&queryParams); err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("query params error: %+v", err),
		})
		return
	}

	rcds, _, err := model.GenerateFrontendApplicationRecords(db, &queryParams, false)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("query result error: %+v", err),
		})
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
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
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
				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
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
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}

		// write first row
		cell, _ := excelize.CoordinatesToCellName(1, 1)
		title := lo.Map(csvHeaderList, func(item string, _ int) any { return item })
		if err := streamWriter.SetRow(cell, title); err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
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
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}

		// write to response
		err = f.Write(ctx.Writer)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		}
	} else if fileFormat == "json" {
		tmpFile, err := os.CreateTemp(os.TempDir(), "application-list-*.json")
		defer os.Remove(tmpFile.Name())

		fileBaseName := filepath.Base(tmpFile.Name())

		jsonBytes, err := json.Marshal(rcds)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}

		err = os.WriteFile(tmpFile.Name(), jsonBytes, 0777)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}

		ctx.FileAttachment(tmpFile.Name(), fileBaseName)
		ctx.Writer.Header().Set("Content-Disposition", `attachment; filename="`+fileBaseName+`"`)
	} else {
		_, _ = ctx.Writer.Write([]byte(""))
	}
}

func DownloadUploadTemplate(ctx *gin.Context) {
	lang := api.GetLangFromQuery(ctx, "en")
	headerStr := internal.ApplicationUploadTemplateHeader[lang]
	if header, found := internal.ApplicationUploadTemplateHeader[lang]; found {
		headerStr = header
	}

	reader := strings.NewReader(headerStr)
	contentLength := len(headerStr)

	extraHeaders := map[string]string{
		"Content-Disposition": `attachment; filename="upload-template.csv"`,
	}

	ctx.DataFromReader(http.StatusOK, int64(contentLength), "encoding/csv", reader, extraHeaders)
}

// Batch operations, most of the request bodies are ids
// For batch process logic, it handles all applications in approved state and process them, so no need to pass ids
// TODO: Those batch actions contain similar logic, check whether it is possible to simplify them

// BatchProcess exports application in approved state, and changes exported applications state to processing
// If there are existing applications in processing state, the export function returns error.
func BatchProcess(ctx *gin.Context) {
	user, enforcer, db, _ := api.ForContext(ctx)

	//  check permission: `(0x..., proj_and_guild, audit_app)`
	ok, err := enforcer.Enforce(user.Wallet, api.ObjProjAndGuild, api.ActAuditApplication)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	var processingRecordCount int64
	db.Model(&model.Application{}).Where("state = ?", model.ApplicationStateProcessing).Count(&processingRecordCount)

	if processingRecordCount > 0 {
		ctx.JSON(http.StatusBadRequest, &api.Reply{
			Code: -1,
			Msg:  "applications in processing state should be processed before exporting new list",
		})
		return
	}

	var applications []model.Application
	err = db.Model(&model.Application{}).Where("state = ?", model.ApplicationStateApproved).Find(&applications).Error
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{Code: -1, Msg: err.Error()})
		return
	}

	push := api.ForContextOnlyPush(ctx)

	err = model.BatchAuditApplication(db, user.Wallet, &applications, model.AuditActionProcess, "", enforcer, push)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("process applications error: %+v", err),
		})
		return
	}

	// TODO: Merge this logic with `BatchComplete`
	appBundleIds := make(map[uint]bool)
	for _, app := range applications {
		appBundleIds[app.BundleId] = true
	}
	err = db.Model(&model.AppBundle{}).Where("id IN ?", lo.Keys(appBundleIds)).Update("state", model.ApplicationStateProcessing).Error
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("process applications error: %+v", err),
		})
		return
	}

	ctx.JSON(http.StatusOK, api.Success(applications))
}

func BatchApprove(ctx *gin.Context) {
	var applications []model.Application
	err := getBatchApplicationsOrReturnError(ctx, &applications)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{Code: -1, Msg: err.Error()})
		return
	}

	user, enforcer, db, _ := api.ForContext(ctx)

	//  check permission: `(0x..., proj_and_guild, audit_app)`
	ok, err := enforcer.Enforce(user.Wallet, api.ObjProjAndGuild, api.ActAuditApplication)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	push := api.ForContextOnlyPush(ctx)
	err = model.BatchAuditApplication(db, user.Wallet, &applications, model.AuditActionApprove, "", enforcer, push)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("approve applications error: %+v", err),
		})
		return
	}

	ctx.JSON(http.StatusOK, "")
}

// BatchReject rejects multiple applications in one API call
func BatchReject(ctx *gin.Context) {
	var applications []model.Application
	err := getBatchApplicationsOrReturnError(ctx, &applications)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{Code: -1, Msg: err.Error()})
		return
	}

	user, enforcer, db, _ := api.ForContext(ctx)

	//  check permission: `(0x..., proj_and_guild, audit_app)`
	ok, err := enforcer.Enforce(user.Wallet, api.ObjProjAndGuild, api.ActAuditApplication)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	push := api.ForContextOnlyPush(ctx)
	err = model.BatchAuditApplication(db, user.Wallet, &applications, model.AuditActionReject, "", enforcer, push)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("reject applications error: %+v", err),
		})
		return
	}

	ctx.JSON(http.StatusOK, "")
}

// BatchComplete completes multiple applications in one API call
// Only applications in processing state can be completed, so no application ids are required for this API call
// This api will fetch all applications with processing state in db and apply `complete` action on them
func BatchComplete(ctx *gin.Context) {
	user, enforcer, db, _ := api.ForContext(ctx)

	//  check permission: `(0x..., proj_and_guild, audit_app)`
	ok, err := enforcer.Enforce(user.Wallet, api.ObjProjAndGuild, api.ActAuditApplication)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	var applications []model.Application
	db.Model(&model.Application{}).Where("state = ?", model.ApplicationStateProcessing).Find(&applications)

	reqBody := AuditRequestBody{}
	err = ctx.Bind(&reqBody)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  "parse request data error",
		})
		return
	}

	push := api.ForContextOnlyPush(ctx)
	err = model.BatchAuditApplication(db, user.Wallet, &applications, model.AuditActionComplete, reqBody.Message, enforcer, push)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("complete applications error: %+v", err),
		})
		return
	}

	appBundleIds := make(map[uint]bool)
	for _, app := range applications {
		appBundleIds[app.BundleId] = true
	}
	err = db.Model(&model.AppBundle{}).Where("id IN ?", lo.Keys(appBundleIds)).Update("state", model.ApplicationStateCompleted).Error
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("process applications error: %+v", err),
		})
		return
	}

	ctx.JSON(http.StatusOK, "")
}

func getBatchApplicationsOrReturnError(ctx *gin.Context, applications *[]model.Application) error {
	var idList []int
	err := ctx.Bind(&idList)
	if err != nil {
		return fmt.Errorf("passed in ID list error")
	}

	db := api.ForContextOnlyDB(ctx)
	db.Find(&applications, idList)
	return nil
}

// Detail returns single application information with requested ID
// GET /applications/{id}
func Detail(ctx *gin.Context) {
	application := &model.Application{}
	getRecordOrReturnNotFound(ctx, application)
	ctx.JSON(http.StatusOK, application)
}

func Approve(ctx *gin.Context) {
	application := &model.Application{}
	auditApplication(ctx, application, model.AuditActionApprove, "")
	ctx.JSON(http.StatusOK, "")
}

func Reject(ctx *gin.Context) {
	application := &model.Application{}
	auditApplication(ctx, application, model.AuditActionReject, "")
	ctx.JSON(http.StatusOK, "")
}

func Process(ctx *gin.Context) {
	application := &model.Application{}
	auditApplication(ctx, application, model.AuditActionProcess, "")
	ctx.JSON(http.StatusOK, "")
}

func Complete(ctx *gin.Context) {
	application := &model.Application{}
	auditApplication(ctx, application, model.AuditActionComplete, "")
	ctx.JSON(http.StatusOK, "")
}

func auditApplication(ctx *gin.Context, application *model.Application, auditAction model.AuditActionType, auditMsg string) {
	getRecordOrReturnNotFound(ctx, application)

	user, enforcer, db, _ := api.ForContext(ctx)
	push := api.ForContextOnlyPush(ctx)

	//  check permission: `(0x..., proj_and_guild, audit_app)`
	ok, err := enforcer.Enforce(user.Wallet, api.ObjProjAndGuild, api.ActAuditApplication)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	if application.ValidateAuditAction(auditAction) {
		err = model.AuditApplication(db, user.Wallet, application, auditAction, auditMsg, enforcer, push)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, api.Reply{
				Code: -1,
				Msg:  fmt.Sprintf("approve application failed, error: %s, please check and resubmit request", err.Error()),
			})
			return
		}
	} else {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("application currently is at state %s, which is not suit for approve", application.State),
		})
		return
	}
}

func getRecordOrReturnNotFound(ctx *gin.Context, application *model.Application) {
	id := ctx.Param("id")
	db := api.ForContextOnlyDB(ctx)
	tx := db.First(&application, id)

	if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
		ctx.JSON(http.StatusNotFound, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("application with id %s not found", id),
		})
		return
	}
}
