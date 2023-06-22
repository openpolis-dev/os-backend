package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

const ApplicationDateQueryFormat = "2006-01-02"

// NewApplicationRecord create application and related audit log message with given params
func NewApplicationRecord(db *gorm.DB, application *Application) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if application.EntityType == "project" {
			project, err := ProjectModel.Detail(db, application.EntityId)
			if err != nil {
				return err
			}

			if project.Status != ProjectStatusOpen {
				return fmt.Errorf("project related applications can only be applied on project in open state, detail : %+v", application)
			}
		} else if application.EntityType == "guild" {
			if application.Type == ApplicationCloseProject {
				return fmt.Errorf("close_project action is not allowed to be applied on guild record, detail: %+v", application)
			}
		} else {
			return fmt.Errorf("unknown entity type, detail : %+v", application)
		}

		if err := tx.Create(application).Error; err != nil {
			return err
		}

		if err := tx.Create(&ApplicationAuditLog{
			ApplicationID: application.ID,
			LogTs:         time.Now(),
			Operation:     AuditActionNew,
			Operator:      application.Applicant,
			PreState:      "",
			PostState:     ApplicationStateOpen,
		}).Error; err != nil {
			return err
		}

		if application.EntityType == "project" && application.Type == ApplicationCloseProject {
			return tx.Model(&Project{ID: application.EntityId}).Update("status", ProjectStatusPendingClose).Error
		}

		return nil
	})
}

func GenerateFrontendApplicationRecordsByIds(db *gorm.DB, ids []uint64) ([]*FrontendApplicationRecord, error) {
	rslt := make([]*FrontendApplicationRecord, len(ids))

	projectRcdsQuerySeg := db.Model(&Application{}).
		Where(&Application{EntityType: "project"}).
		Where("applications.id IN ?", ids).
		Joins("inner join projects on projects.id = applications.entity_id").
		Select(jointAppProjectFields)
	// TODO: Guild has not implemented yet
	//guildRecords := db.Model(&Application{}).Where(&Application{EntityType: "guild"}).Joins("inner join guilds on guilds.id = applications.entity_id").Select(jointAppProjectFields)

	projectRcds, err := gormfind.RowsJoin[jointAppProjectRslt](projectRcdsQuerySeg, "applications", nil)
	if err != nil {
		return nil, err
	}

	for i, r := range projectRcds {
		rslt[i] = r.ToFrontedApplicationRecord(db)
	}

	return rslt, nil
}

func GenerateFrontendApplicationRecords(db *gorm.DB, queryParams *ListApplicationQueryParams) ([]*FrontendApplicationRecord, int64, error) {
	clearAppType := strings.ToLower(strings.TrimSpace(queryParams.Type))
	clearEntity := strings.ToLower(strings.TrimSpace(queryParams.Entity))
	clearState := strings.ToLower(strings.TrimSpace(queryParams.State))

	if !lo.Contains([]string{"close_project", "new_reward"}, clearAppType) {
		return nil, 0, fmt.Errorf("unknown application type %s", queryParams.Type)
	}

	if !lo.Contains([]string{"project", "guild"}, clearEntity) {
		return nil, 0, fmt.Errorf("unknown entity type %s", queryParams.Entity)
	}

	appType := MustParseApplicationType(queryParams.Type)
	querySeg := db.Model(&Application{}).Where(&Application{Type: appType, EntityType: clearEntity})

	if queryParams.Applicant != "" {
		querySeg = querySeg.Where(&Application{Applicant: queryParams.Applicant})
	}

	if queryParams.State != "" {
		if !lo.Contains([]string{"open", "approved", "rejected", "processing", "completed"}, clearState) {
			return nil, 0, fmt.Errorf("unknown state %s", queryParams.State)
		}
		querySeg = querySeg.Where(&Application{State: ApplicationState(clearState)})
	}

	if queryParams.StartDate != "" && queryParams.EndDate != "" {
		startDate, err := time.Parse(ApplicationDateQueryFormat, queryParams.StartDate)
		if err != nil {
			return nil, 0, err
		}

		endDate, err := time.Parse(ApplicationDateQueryFormat, queryParams.EndDate)
		if err != nil {
			return nil, 0, err
		}

		querySeg = querySeg.Where("created_at >= ? AND created_at <= ?", startDate, endDate)
	}

	switch clearEntity {
	case "project":
		querySeg = querySeg.Joins("inner join projects on projects.id = applications.entity_id").Select(jointAppProjectFields)
	case "guild":
		// TODO: guild is not implemented yet
		querySeg = querySeg.Joins("inner join projects on projects.id = applications.entity_id").Select(jointAppProjectFields)
	}

	if len(strings.TrimSpace(queryParams.EntityId)) != 0 {
		entityId, err := strconv.ParseUint(queryParams.EntityId, 10, 64)
		if err != nil {
			return nil, 0, err
		}
		switch queryParams.Entity {
		case "project":
			querySeg = querySeg.Where(&Project{ID: uint(entityId)})
		case "guild":
			// TODO: Not implemented yet, use project for example
			querySeg = querySeg.Where(&Project{ID: uint(entityId)})
		}
	}

	if queryParams.SortField == "" {
		queryParams.SortField = "created_at"
	}

	if queryParams.Size == 0 {
		queryParams.Size = api.DefaultPageSize
	}

	gormFindPage := gormfind.Page{
		Page:      queryParams.Page,
		Size:      queryParams.Size,
		SortField: &queryParams.SortField,
		Order:     &queryParams.SortOrder,
	}

	total, err := gormfind.Count(querySeg)
	if err != nil {
		return nil, 0, err
	}

	rcds, err := gormfind.RowsJoin[jointAppProjectRslt](querySeg, "applications", &gormFindPage)
	if err != nil {
		return nil, 0, err
	}

	rslt := make([]*FrontendApplicationRecord, len(rcds))

	for i, r := range rcds {
		rslt[i] = r.ToFrontedApplicationRecord(db)
	}

	return rslt, total, nil
}
