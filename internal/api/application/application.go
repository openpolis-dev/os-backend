package application

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

type AuditRequestBody struct {
	Message string `json:"message"`
}

// NewApplicationRequest is used to save new application request data passed from frontend
type NewApplicationRequest struct {
	Type             string `json:"type"`
	Entity           string `json:"entity"`
	EntityId         uint   `json:"entity_id"`
	TargetUserWallet string `json:"target_user_wallet"`
	CreditAmount     uint64 `json:"credit_amount"`
	TokenAmount      uint64 `json:"token_amount"`
	DetailedType     string `json:"detailed_type"`
	Comment          string `json:"comment"`
}

// ListApplicants list all applicants existing in applications table for filter
func ListApplicants(ctx *gin.Context) {
	var err error
	db := api.ForContextOnlyDB(ctx)

	var rslt []struct {
		Applicant string
		Name      string
	}

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
// GET /applications
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

	rcds, total, err := model.GenerateFrontendApplicationRecords(db, &queryParams)
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
// POST /applications/
func Create(ctx *gin.Context) {
	var newApplicationReqs []NewApplicationRequest
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

			app := &model.Application{
				Type:         appType,
				Applicant:    user.Wallet,
				State:        model.ApplicationStateOpen,
				EntityType:   req.Entity,
				EntityId:     req.EntityId,
				DetailedType: req.DetailedType,
				Comment:      req.Comment,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			}

			if appType == model.ApplicationNewReward {
				rewardDetailedData := model.NewRewardApplicationDetailedData{
					model.BudgetTypeCredit: {
						TargetUserWallet: req.TargetUserWallet,
						AssetType:        model.BudgetTypeCredit,
						Amount:           req.CreditAmount,
					},
					model.BudgetTypeToken: {
						TargetUserWallet: req.TargetUserWallet,
						AssetType:        model.BudgetTypeToken,
						Amount:           req.TokenAmount,
					},
				}

				detailedDataBytes, err := json.Marshal(rewardDetailedData)
				if err != nil {
					return err
				}

				app.DetailedData = detailedDataBytes
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

// Download get lists from passed in IDs and generate file and send to invoker
func Download(ctx *gin.Context) {
	fileFormat := ""
	fileFormat = strings.ToLower(ctx.Query("format"))
	if fileFormat == "" {
		fileFormat = "csv"
	}

	db := api.ForContextOnlyDB(ctx)

	idList := ctx.Query("ids")

	if len(idList) == 0 {
		ctx.JSON(http.StatusBadRequest,
			api.ServerError(fmt.Errorf("pass application id in ids query param with format 1,2,3,4")))
		return
	}

	var err error
	err = nil

	ids := lo.Map(strings.Split(idList, ","), func(idStr string, _ int) uint64 {
		val, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			err = fmt.Errorf("invalid application id %s", idStr)
			return 0
		}
		return val
	})

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
	}

	rcds, err := model.GenerateFrontendApplicationRecordsByIds(db, ids)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
	}

	if fileFormat == "csv" {
		tmpFile, err := os.CreateTemp(os.TempDir(), "application-list-*.csv")
		defer os.Remove(tmpFile.Name())

		fileBaseName := filepath.Base(tmpFile.Name())

		w := csv.NewWriter(tmpFile)
		err = w.Write(model.FrontendApplicationRecordCsvHeader)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		}
		for _, r := range rcds {
			err = w.Write(r.ToCSV())
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			}
		}
		w.Flush()

		ctx.FileAttachment(tmpFile.Name(), fileBaseName)
		ctx.Writer.Header().Set("Content-Disposition", `attachment; filename="`+fileBaseName+`"`)
	} else if fileFormat == "json" {
		tmpFile, err := os.CreateTemp(os.TempDir(), "application-list-*.json")
		defer os.Remove(tmpFile.Name())

		fileBaseName := filepath.Base(tmpFile.Name())

		jsonBytes, err := json.Marshal(rcds)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		}

		err = os.WriteFile(tmpFile.Name(), jsonBytes, 0777)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		}

		ctx.FileAttachment(tmpFile.Name(), fileBaseName)
		ctx.Writer.Header().Set("Content-Disposition", `attachment; filename="`+fileBaseName+`"`)
	} else {
		_, _ = ctx.Writer.Write([]byte(""))
	}
}

func DownloadUploadTemplate(ctx *gin.Context) {
	lang := ctx.Query("lang")
	headerStr := api.ApplicationUploadTemplateHeader["en"]
	if header, found := api.ApplicationUploadTemplateHeader[lang]; found {
		headerStr = header
	}

	reader := strings.NewReader(headerStr)
	contentLength := len(headerStr)

	extraHeaders := map[string]string{
		"Content-Disposition": `attachment; filename="upload-template.csv"`,
	}

	ctx.DataFromReader(http.StatusOK, int64(contentLength), "encoding/csv", reader, extraHeaders)
}

// Batch operations, the request body are ids
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
	err = getBatchApplicationsOrReturnError(ctx, &applications)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{Code: -1, Msg: err.Error()})
		return
	}

	err = model.BatchAuditApplication(db, user.Wallet, &applications, model.AuditActionProcess, "")
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

	err = model.BatchAuditApplication(db, user.Wallet, &applications, model.AuditActionApprove, "")
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

	err = model.BatchAuditApplication(db, user.Wallet, &applications, model.AuditActionReject, "")
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

	err = model.BatchAuditApplication(db, user.Wallet, &applications, model.AuditActionComplete, reqBody.Message)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("complete applications error: %+v", err),
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
	notificator := api.ForContextOnlyNotificator(ctx)

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
		err = model.AuditApplication(db, user.Wallet, application, auditAction, auditMsg, enforcer, notificator)
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
