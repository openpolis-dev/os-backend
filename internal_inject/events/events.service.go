package events_inject

import (
	"errors"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type EventsService struct {
	// inject

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`
}

func (s *EventsService) GetRecord(db *gorm.DB, idStr string) (*model.Event, error) {
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return nil, err
	}

	querySeg := db.Where("id = ?", id)
	return gormfind.Row[model.Event](querySeg)
}

func (s *EventsService) UpdateQuerySegByState(ctx *gin.Context, querySeg *gorm.DB) (*gorm.DB, error) {
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

func (s *EventsService) GetMultipleRecords(page *gormfind.Page, querySeg *gorm.DB) (*api.ListReplyData, error) {
	total, err := gormfind.Count(querySeg)
	if err != nil {
		return nil, err
	}

	records, err := model.QueryRows[model.Event](querySeg, page)
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

func (s *EventsService) ParseTimestampStrToTime(timeStr string) (tsObject time.Time, err error) {
	i, err := strconv.ParseInt(timeStr, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(i, 0), nil
}

func (s *EventsService) UpdateEventFromRequest(eventRecord *model.Event, req CreateOrUpdateReq) error {
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
		startTime, err := s.ParseTimestampStrToTime(req.StartAt)
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
		endTime, err := s.ParseTimestampStrToTime(req.EndAt)
		if err != nil {
			return err
		}

		if endTime.Before(time.Now()) || endTime.Before(eventRecord.StartAt) {
			return errors.New("invalid end time")
		}
		eventRecord.EndAt = endTime
	}

	if req.Metadata != "" {
		eventRecord.Metadata = req.Metadata
	}

	return nil
}
