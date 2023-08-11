package event

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type (
	CreateOrUpdateReq struct {
		Title    string `json:"title"`
		CoverImg string `json:"cover_img"`
		Content  string `json:"content"`

		StartAt string `json:"start_at"`
		EndAt   string `json:"end_at"`

		Metadata string `json:"metadata"`
	}
)

// List `GET /events?status=open&page=1&size=10&sort_field=created_at&sort_order=desc`
func List(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)
	page := api.ParseAndConvertPageParam(ctx)
	querySeg := db.Model(model.Event{})
	querySeg, err := updateQuerySegByState(ctx, querySeg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.BadRequest(err))
		return
	}

	listReplyData, err := getMultipleRecords(page, querySeg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.BadRequest(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(listReplyData))
}

func MyList(ctx *gin.Context) {
	user, db := api.ForContextUserAndDB(ctx)
	page := api.ParseAndConvertPageParam(ctx)
	querySeg := db.Model(model.Event{}).Where(model.Event{Initiator: strings.ToLower(user.Wallet)})
	querySeg, err := updateQuerySegByState(ctx, querySeg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.BadRequest(err))
		return
	}

	listReplyData, err := getMultipleRecords(page, querySeg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.BadRequest(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(listReplyData))
}

func Create(ctx *gin.Context) {
	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(user.Wallet, api.ObjEvent, api.ActCreateEvent)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
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

	startDate, err := parseTimestampStrToTime(req.StartAt)
	endDate, err := parseTimestampStrToTime(req.EndAt)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("invalid start or end date")))
		return
	}

	if endDate.Before(startDate) || startDate.Before(time.Now()) || endDate.Before(time.Now()) {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("invalid start or end date")))
		return
	}

	eventRecord := model.Event{
		Initiator: strings.ToLower(user.Wallet),
		Title:     req.Title,
		CoverImg:  req.CoverImg,
		Content:   req.Content,
		StartAt:   startDate,
		EndAt:     endDate,
		Metadata:  req.Metadata,
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
	patterStr := ctx.Query("delete_key")
	if patterStr != api.EventDeleteMagicWorld {
		ctx.JSON(http.StatusNotFound, "")
		return
	}

	eventRecord, err := getRecord(db, ctx.Param("id"))
	err = db.Delete(&eventRecord).Error
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	ctx.JSON(http.StatusOK, api.Success(eventRecord))
}

func Update(ctx *gin.Context) {
	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(user.Wallet, api.ObjEvent, api.ActCreateEvent)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	req := CreateOrUpdateReq{}
	err = ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

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

func getRecord(db *gorm.DB, idStr string) (*model.Event, error) {
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return nil, err
	}

	querySeg := db.Where("id = ?", id)
	return gormfind.Row[model.Event](querySeg)
}

func updateQuerySegByState(ctx *gin.Context, querySeg *gorm.DB) (*gorm.DB, error) {
	stateStr := ctx.Query("state")
	if stateStr == "" {
		return querySeg, nil
	}

	state := model.EventState(stateStr)

	if state == model.EventStatePrepare {
		querySeg = querySeg.Where("created_at >= ?", time.Now().Unix())
	} else if state == model.EventStateInProgress {
		querySeg = querySeg.Where("created_at < ? AND end_at > ?", time.Now().Unix(), time.Now().Unix())
	} else if state == model.EventStateCompleted {
		querySeg = querySeg.Where("end_at < ?", time.Now().Unix())
	} else {
		return querySeg, errors.New("unknown state")
	}

	return querySeg, nil
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

func parseTimestampStrToTime(timeStr string) (tsObject time.Time, err error) {
	i, err := strconv.ParseInt(timeStr, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(i, 0), nil
}

func updateEventFromRequest(eventRecord *model.Event, req CreateOrUpdateReq) error {
	// TODO: How to handle delete field request? And what fields are required and not allowed to be cleared?
	if req.Title != "" {
		eventRecord.Title = req.Title
	}
	if req.Content != "" {
		eventRecord.Content = req.Content
	}

	if req.CoverImg != "" {
		eventRecord.CoverImg = req.CoverImg
	}

	// Verify startDate is later than today if have
	if req.StartAt != "" {
		startTime, err := parseTimestampStrToTime(req.StartAt)
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
		endTime, err := parseTimestampStrToTime(req.EndAt)
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
