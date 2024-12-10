package events_inject

import (
	"errors"
	"net/http"
	"time"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type EventsController struct {
	// inject

	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	EventsService *EventsService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var events EventsController

	err := inject.Populate(&events, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var eventsGroup *gin.RouterGroup
	var eventsAuthGroup *gin.RouterGroup

	if fatherGroup != nil {
		eventsGroup = fatherGroup.Group("/events")
		eventsAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/events")
	} else {
		eventsGroup = events.Gin.Group("/events")
		eventsAuthGroup = events.Gin.Group("/", middleware.AuthRequired).Group("/events")
	}

	// no auth
	eventsGroup.GET("/", events.List)
	eventsGroup.GET("/:id", events.Detail)

	// auth
	eventsAuthGroup.POST("/", events.Create)
	eventsAuthGroup.PUT("/:id", events.Update)
	eventsAuthGroup.DELETE("/:id", events.Delete)
	// my guilds
	if fatherGroup != nil {
		fatherGroup.Group("/", middleware.AuthRequired).GET("/my_events", events.MyList)
	} else {
		events.Gin.Group("/", middleware.AuthRequired).GET("/my_events", events.MyList)
	}
}

func (c *EventsController) List(ctx *gin.Context) {
	page := api.ParseAndConvertPageParam(ctx)
	querySeg := c.Db.Model(model.Event{})
	querySeg, err := c.EventsService.UpdateQuerySegByState(ctx, querySeg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.BadRequest(err))
		return
	}

	listReplyData, err := c.EventsService.GetMultipleRecords(page, querySeg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.BadRequest(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(listReplyData))
}

func (c *EventsController) Detail(ctx *gin.Context) {
	eventRecord, err := c.EventsService.GetRecord(c.Db, ctx.Param("id"))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get event error")))
		return
	}
	if eventRecord == nil {
		ctx.JSON(http.StatusNotFound, nil)
		return
	}

	ctx.JSON(http.StatusOK, api.Success(eventRecord))
}

func (c *EventsController) MyList(ctx *gin.Context) {
	user, db := api.ForContextUserAndDB(ctx)
	page := api.ParseAndConvertPageParam(ctx)
	querySeg := db.Model(model.Event{}).Where(model.Event{Initiator: common.FormatUserWallet(user.Wallet)})
	querySeg, err := c.EventsService.UpdateQuerySegByState(ctx, querySeg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.BadRequest(err))
		return
	}

	listReplyData, err := c.EventsService.GetMultipleRecords(page, querySeg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.BadRequest(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(listReplyData))
}

func (c *EventsController) Create(ctx *gin.Context) {
	user, enforcer, _, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), internal.ObjEvent, internal.ActCreateEvent)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.ObjEvent, internal.ActCreateEvent)
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	req := CreateOrUpdateReq{}
	err = ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	if req.StartAt == "" {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("invalid start time")))
		return
	}

	if req.EndAt == "" {
		req.EndAt = req.StartAt
	}

	startDate, _ := c.EventsService.ParseTimestampStrToTime(req.StartAt)
	endDate, err := c.EventsService.ParseTimestampStrToTime(req.EndAt)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("invalid start or end date")))
		return
	}

	if endDate.Before(startDate) || startDate.Before(time.Now()) || endDate.Before(time.Now()) {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("invalid start or end date")))
		return
	}

	eventRecord := model.Event{
		Initiator: common.FormatUserWallet(user.Wallet),
		Title:     req.Title,
		CoverImg:  req.CoverImg,
		Content:   req.Content,
		StartAt:   startDate,
		EndAt:     endDate,
		Metadata:  req.Metadata,
		CreatedAt: time.Now().In(internal.ProjectTimezone),
		UpdatedAt: time.Now().In(internal.ProjectTimezone),
		CreateTs:  model.GetCurrentUtcEpochSecond(),
		UpdateTs:  model.GetCurrentUtcEpochSecond(),
	}

	err = c.Db.Create(&eventRecord).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create event error")))
	} else {
		ctx.JSON(http.StatusCreated, api.Success(eventRecord))
	}
}

func (c *EventsController) Update(ctx *gin.Context) {
	user, enforcer, _, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), internal.ObjEvent, internal.ActCreateEvent)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.ObjEvent, internal.ActCreateEvent)
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	req := CreateOrUpdateReq{}
	err = ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	eventRecord, err := c.EventsService.GetRecord(c.Db, ctx.Param("id"))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get event error")))
		return
	}

	if eventRecord == nil {
		ctx.JSON(http.StatusNotFound, nil)
		return
	}

	if eventRecord.StartAt.Before(time.Now()) {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("updating started event is not allowed")))
		return
	}
	err = c.EventsService.UpdateEventFromRequest(eventRecord, req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	eventRecord.UpdatedAt = time.Now().In(internal.ProjectTimezone)
	eventRecord.UpdateTs = model.GetCurrentUtcEpochSecond()
	err = c.Db.Save(&eventRecord).Error
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.BadRequest(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(eventRecord))
}

func (c *EventsController) Delete(ctx *gin.Context) {
	patterStr := ctx.Query("delete_key")
	if patterStr != internal.EventDeleteMagicWorld {
		ctx.JSON(http.StatusNotFound, "")
		return
	}

	eventRecord, _ := c.EventsService.GetRecord(c.Db, ctx.Param("id"))
	err := c.Db.Delete(&eventRecord).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("delete event error")))
		return
	}
	ctx.JSON(http.StatusOK, api.Success(eventRecord))
}
