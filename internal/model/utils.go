package model

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/api"
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

func GenerateFrontendApplicationRecordsByIds(db *gorm.DB, ids []uint64) ([]*FrontendApplicationRecord, error) {
	rslt := make([]*FrontendApplicationRecord, len(ids))

	return rslt, nil
}

func GenerateFrontendApplicationRecords(db *gorm.DB, queryParams *ListApplicationQueryParams) ([]*FrontendApplicationRecord, int64, error) {
	clearAppType := strings.ToLower(strings.TrimSpace(queryParams.Type))
	clearEntity := strings.ToLower(strings.TrimSpace(queryParams.Entity))
	if !lo.Contains([]string{"close_project", "new_reward"}, clearAppType) {
		return nil, 0, fmt.Errorf("unknown application type %s", queryParams.Type)
	}

	if !lo.Contains([]string{"project", "guild"}, clearEntity) {
		return nil, 0, fmt.Errorf("unknown entity type %s", queryParams.Entity)
	}

	appType := MustParseApplicationType(queryParams.Type)
	querySeg := db.Model(&Application{}).Where(&Application{Type: appType, EntityType: clearEntity})

	switch clearEntity {
	case "project":
		querySeg = querySeg.Joins("inner join projects on projects.id = applications.entity_id").Select(jointAppProjectFields)
	case "guild":
		querySeg = querySeg.Select("guilds.name as entity_name").Joins("left join guilds on projects.id = applications.entity_id")
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
		var tokenAmount uint64
		var creditAmount uint64
		var targetUserWallet string
		if r.Application.Type == ApplicationNewReward {
			detailedData := NewRewardApplicationDetailedData{}
			err := json.Unmarshal(r.Application.DetailedData, &detailedData)
			if err != nil {
				return nil, 0, err
			}

			tokenAmount, _ = detailedData.AmountOfBudgetType(BudgetTypeToken)
			creditAmount, _ = detailedData.AmountOfBudgetType(BudgetTypeCredit)

			targetUserWallet = detailedData.GetTargetUserWallet()
		}

		var submitterWallet string
		var submitterUsername string
		var reviewerWallet string
		var reviewerUsername string

		submitterWallet = r.Application.Applicant
		submitterUsername, err = UserModel.TryGetUsername(db, submitterWallet)
		if err != nil {
			return nil, 0, err
		}

		auditlog := ApplicationAuditLog{}
		err = db.Model(&ApplicationAuditLog{ApplicationID: r.Application.ID}).
			Where("operation = ?", AuditActionApprove).Or("operation = ?", AuditActionReject).First(&auditlog).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				// No record found, skip
			} else {
				return nil, 0, err
			}
		} else {
			reviewerWallet = auditlog.Operator
			reviewerUsername, err = UserModel.TryGetUsername(db, reviewerWallet)
			if err != nil {
				return nil, 0, err
			}
		}

		rslt[i] = &FrontendApplicationRecord{
			ApplicationID:    r.Application.ID,
			EntityName:       clearEntity,
			CreatedAt:        r.Application.CreatedAt,
			TargetUserWallet: targetUserWallet,
			TokenAmount:      tokenAmount,
			CreditAmount:     creditAmount,
			BudgetSource:     r.Project.Name,
			Status:           string(r.Application.State),
			SubmitterWallet:  submitterWallet,
			SubmitterName:    submitterUsername,
			ReviewerWallet:   reviewerWallet,
			ReviewerName:     reviewerUsername,
			TransactionIds:   r.Application.CompleteMessage,
		}
	}

	return rslt, total, nil
}
