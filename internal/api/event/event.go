package event

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type (
	CreateOrUpdateReq struct {
		Title   string `json:"title"`
		Content string `json:"content"`

		Initiator string `json:"initiator"`
		StartAt   string `json:"start_at"`
		EndAt     string `json:"end_at"`

		Metadata string `json:"metadata"`
	}
)

// List `GET /events?status=open&page=1&size=10&sort_field=created_at&sort_order=desc`
func List(ctx *gin.Context) {
	var err error
	db := api.ForContextOnlyDB(ctx)
	page := api.ParseAndConvertPageParam(ctx)
	querySeg := db.Model(model.Event{})

	listReplyData, err := getMultipleRecords(page, querySeg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.BadRequest(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(listReplyData))
}

func Create(ctx *gin.Context) {
	req := CreateOrUpdateReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	if req.StartAt == "" {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("invalid start time")))
		return
	}

	startDate, endDate, err := parseStartEndDate(req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("invalid start or end date")))
		return
	}

	// TODO: Verify the date time check for EndAt not passed cases
	dateIsValid := isDateValid(startDate, endDate)
	if !dateIsValid {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("invalid start or end date")))
		return
	}

	db := api.ForContextOnlyDB(ctx)
	eventRecord := model.Event{
		Initiator: req.Initiator,
		Title:     req.Title,
		Content:   req.Content,
		StartAt:   startDate,
		EndAt:     endDate,
		State:     model.EventStatePrepare,
	}

	err = db.Create(&eventRecord).Error
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
	} else {
		ctx.JSON(http.StatusCreated, api.Success(eventRecord))
	}
}

func Detail(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)
	eventRecord, err := getRecord(db, ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}
	ctx.JSON(http.StatusOK, api.Success(eventRecord))
}

func Update(ctx *gin.Context) {
	req := CreateOrUpdateReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	db := api.ForContextOnlyDB(ctx)
	eventRecord, err := getRecord(db, ctx.Param("id"))
	if eventRecord.StartAt.Before(time.Now()) {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("updating started event is not allowed")))
		return
	}
	err = updateEventFromRequest(eventRecord, req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	err = db.Save(&eventRecord).Error
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.BadRequest(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(eventRecord))
}

func MyList(ctx *gin.Context) {
	user, db := api.ForContextUserAndDB(ctx)
	page := api.ParseAndConvertPageParam(ctx)
	querySeg := db.Where(model.Event{Initiator: user.Wallet})

	listReplyData, err := getMultipleRecords(page, querySeg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.BadRequest(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(listReplyData))
}

func getRecord(db *gorm.DB, idStr string) (*model.Event, error) {
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return nil, err
	}

	querySeg := db.Where("id = ?", id)
	return gormfind.Row[model.Event](querySeg)
}

func getMultipleRecords(page *gormfind.Page, querySeg *gorm.DB) (*api.ListReplyData, error) {
	total, err := gormfind.Count(querySeg)
	if err != nil {
		return nil, err
	}

	records, err := gormfind.Rows[model.Event](querySeg, page)
	if err != nil {
		return nil, err
	}

	return &api.ListReplyData{
		Page:  page.Page,
		Size:  page.Size,
		Total: total,
		Rows:  records,
	}, err
}

func parseStartEndDate(req CreateOrUpdateReq) (startDate time.Time, endDate time.Time, err error) {
	startDate, err = time.Parse(req.StartAt, model.DateQueryFormat)
	if err != nil {
		return
	}
	if req.EndAt != "" {
		endDate, err = time.Parse(req.EndAt, model.DateQueryFormat)
		if err != nil {
			return
		}
	}
	return
}

func isDateValid(startDate, endDate time.Time) bool {
	return startDate.Before(endDate) && startDate.Before(time.Now()) && endDate.Before(time.Now())
}

func updateEventFromRequest(eventRecord *model.Event, req CreateOrUpdateReq) error {
	// TODO: How to handle delete field request? And what fields are required and not allowed to be cleared?
	if req.Title != "" {
		eventRecord.Title = req.Title
	}
	if req.Content != "" {
		eventRecord.Content = req.Content
	}

	// Verify startDate is later than today if have
	if req.StartAt != "" {
		startTime, err := time.Parse(req.StartAt, model.DateTimeFormat)
		if err != nil {
			return err
		}

		if startTime.Before(time.Now()) {
			return errors.New("invalid start time")
		}
		eventRecord.StartAt = startTime
	}

	// Verify endDate is later than today and startDate
	if req.EndAt != "" {
		endTime, err := time.Parse(req.EndAt, model.DateTimeFormat)
		if err != nil {
			return err
		}

		if endTime.Before(time.Now()) || endTime.Before(eventRecord.StartAt) {
			return errors.New("invalid end time")
		}
		eventRecord.EndAt = endTime
	}

	return nil
}
