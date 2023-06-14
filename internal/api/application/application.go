package application

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

var err error

type AuditRequestBody struct {
	Message string `json:"message"`
}

// NewApplicationRequest is used to save new application request data passed from frontend
// TODO: Verify with frontend side about passed in params
type NewApplicationRequest struct {
	Type             string `json:"type"`
	Entity           string `json:"entity"`
	EntityId         string `json:"entity_id"`
	TargetUserWallet string `json:"target_user_wallet"`
	AssetName        string `json:"asset_name"`
	Amount           uint64 `json:"amount"`
}

// List lists all applications based on query params and return in JSON format
// POST /applications
func List(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)

	page := api.ParseAndConvertPageParam(ctx)

	querySeg := db.Model(&model.Application{})
	rcds, err := gormfind.Rows[model.Application](querySeg, page)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.Reply{
			Code: -1,
			Msg:  "query error",
		})
	}
	ctx.JSON(http.StatusOK, api.Success(rcds))
}

// Create handles creating application with passed in data
// An audit log record will be created with application at same time with action open
// POST /applications
func Create(ctx *gin.Context) {
	user, _, db, _ := api.ForContext(ctx)

	newApplicationReq := NewApplicationRequest{}
	if err := ctx.BindJSON(newApplicationReq); err != nil {
		if err != nil {
			ctx.JSON(http.StatusBadRequest, api.Reply{
				Code: -1,
				Msg:  "Data error",
			})
		}
	}

	if newApplicationReq.Type != "close_project" && newApplicationReq.Type != "new_reward" {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("unknown application type %s", newApplicationReq.Type),
		})
	}

	if newApplicationReq.Entity != "guild" && newApplicationReq.Entity != "project" {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("unknown application entity %s", newApplicationReq.Entity),
		})
	}

	application := model.Application{
		DisplayGroupId: "",
		Type:           model.ApplicationType(newApplicationReq.Type),
		Applicant:      user.Wallet,
		State:          model.ApplicationStateOpen,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		DetailedData: model.ApplicationDetailedData{
			TargetUserWallet: newApplicationReq.TargetUserWallet,
			AssetName:        newApplicationReq.AssetName,
			Amount:           newApplicationReq.Amount,
		},
		EntityType: "",
		EntityId:   0,
		AuditLogs:  nil,
	}

	// Update applicant data
	application.Applicant = user.Wallet

	err = model.NewApplicationRecord(db, &application)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("creation application object error, %+v", err),
		})
	}

	ctx.JSON(http.StatusCreated, api.Success(&application))
}

// Batch operations, the request body are ids

// Export exports application in approved state, and changes exported applications state to processing
// If there are existing applications in processing state, the export function returns error.
// Actually, Export is the batchProcess operation
func Export(ctx *gin.Context) {
	user, _, db, _ := api.ForContext(ctx)
	var processingRecordCount int64
	db.Model(&model.Application{}).Where("state <> ?", model.ApplicationStateProcessing).Count(&processingRecordCount)

	if processingRecordCount > 0 {
		ctx.JSON(http.StatusBadRequest, &api.Reply{
			Code: -1,
			Msg:  "applications in processing state should be processed before exporting new list",
		})
	}

	var applications []model.Application
	getBatchApplicationsOrReturnError(ctx, &applications)

	err = model.BatchAuditApplication(db, user.Wallet, &applications, model.AuditActionProcess, "")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("process applications error: %+v", err),
		})
	}

	ctx.JSON(http.StatusOK, api.Success(applications))
}

func BatchApprove(ctx *gin.Context) {
	var applications []model.Application
	getBatchApplicationsOrReturnError(ctx, &applications)

	user, _, db, _ := api.ForContext(ctx)
	err = model.BatchAuditApplication(db, user.Wallet, &applications, model.AuditActionApprove, "")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("approve applications error: %+v", err),
		})
	}

	ctx.JSON(http.StatusOK, "")
}

// BatchReject rejects multiple applications in one API call
func BatchReject(ctx *gin.Context) {
	var applications []model.Application
	getBatchApplicationsOrReturnError(ctx, &applications)

	user, _, db, _ := api.ForContext(ctx)
	err = model.BatchAuditApplication(db, user.Wallet, &applications, model.AuditActionReject, "")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("reject applications error: %+v", err),
		})
	}

	ctx.JSON(http.StatusOK, "")
}

// BatchComplete completes multiple applications in one API call
// Only applications in processing state can be completed, so no application ids are required for this API call
// This api will fetch all applications with processing state in db and apply `complete` action on them
func BatchComplete(ctx *gin.Context) {
	user, _, db, _ := api.ForContext(ctx)
	var applications []model.Application
	db.Model(&model.Application{}).Where("state = ?", model.ApplicationStateProcessing).Find(&applications)

	reqBody := AuditRequestBody{}
	err := ctx.Bind(&reqBody)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  "parse request data error",
		})
	}

	err = model.BatchAuditApplication(db, user.Wallet, &applications, model.AuditActionComplete, reqBody.Message)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("complete applications error: %+v", err),
		})
	}

	ctx.JSON(http.StatusOK, "")
}

func getBatchApplicationsOrReturnError(ctx *gin.Context, applications *[]model.Application) {
	var idList []int
	err := ctx.Bind(&idList)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{Code: -1, Msg: "passed in ID list error"})
	}

	db := api.ForContextOnlyDB(ctx)
	db.Find(&applications, idList)
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

	user, _, db, _ := api.ForContext(ctx)
	if application.ValidateAuditAction(auditAction) {
		err = model.AuditApplication(db, user.Wallet, application, auditAction, auditMsg)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, api.Reply{
				Code: -1,
				Msg:  fmt.Sprintf("approve application failed, error: %s, please check and resubmit request", err.Error()),
			})
		}
	} else {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("application currently is at state %s, which is not suit for approve", application.State),
		})
	}
}

func getRecordOrReturnNotFound(ctx *gin.Context, application *model.Application) {
	id := ctx.Param("id")
	db := api.ForContextOnlyDB(ctx)
	tx := db.First(&application, id)

	if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
		ctx.JSON(http.StatusNotFound, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("application with id %d not found", id),
		})
	}
}
