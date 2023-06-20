package model

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

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

func GenerateFrontendApplicationRecords(db *gorm.DB, queryParams *ListApplicationQueryParams) ([]*FrontendApplicationRecord, int64, error) {
	if !lo.Contains([]string{"close_project", "new_reward"}, strings.ToLower(strings.TrimSpace(queryParams.Type))) {
		return nil, 0, fmt.Errorf("unknown application type %s", queryParams.Type)
	}

	if !lo.Contains([]string{"project", "guild"}, strings.ToLower(strings.TrimSpace(queryParams.Entity))) {
		return nil, 0, fmt.Errorf("unknown entity type %s", queryParams.Entity)
	}

	appType := MustParseApplicationType(queryParams.Type)
	querySeg := db.Preload(queryParams.Entity).Model(&Application{}).Where("type = ?", appType)

	if len(strings.TrimSpace(queryParams.EntityId)) != 0 {
		entityId, err := strconv.Atoi(queryParams.EntityId)
		if err != nil {
			return nil, 0, err
		}
		querySeg = querySeg.Where(fmt.Sprintf("`%s.id = ?`", queryParams.Entity), entityId)
	}

	fmt.Printf("query seg: %+v\n", querySeg)

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

	rcds, err := gormfind.RowsJoin[FrontendApplicationRecord](querySeg, "application", &gormFindPage)
	if err != nil {
		return nil, 0, err
	}

	return rcds, total, nil
}
