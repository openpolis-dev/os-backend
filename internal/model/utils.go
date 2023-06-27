package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

const ApplicationDateQueryFormat = "2006-01-02"

const QueryApplicationsWithEntityNameBaseSQL = `SELECT applications.*,
CASE
   WHEN applications.entity_type = 'project' THEN projects.name
   WHEN applications.entity_type = 'guild' THEN guilds.name
   ELSE NULL END AS entity_name
FROM applications
   LEFT JOIN projects ON applications.entity_type = 'project' AND applications.entity_id = projects.id
   LEFT JOIN guilds ON applications.entity_type = 'guild' AND applications.entity_id = guilds.id`

// NewApplicationRecord create application and related audit log message with given params
func NewApplicationRecord(db *gorm.DB, application *Application) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if application.EntityType == "project" {
			project, err := ProjectModel.Detail(db, application.EntityId)
			if err != nil {
				return err
			}
			if project == nil {
				return fmt.Errorf("project with id %d not found", application.EntityId)
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
			return tx.Model(&Project{}).Where(&Project{ID: application.EntityId}).Update("status", ProjectStatusPendingClose).Error
		}

		return nil
	})
}

func GenerateFrontendApplicationRecordsByIds(db *gorm.DB, ids []uint64) ([]*FrontendApplicationRecord, error) {

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

	rslt := make([]*FrontendApplicationRecord, len(projectRcds))
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

	if clearEntity != "" {
		if !lo.Contains([]string{"project", "guild"}, clearEntity) {
			return nil, 0, fmt.Errorf("unknown entity type %s", queryParams.Entity)
		}
	}

	appType := MustParseApplicationType(queryParams.Type)

	querySQL := QueryApplicationsWithEntityNameBaseSQL
	whereClause := "\nWHERE applications.type = @app_type"
	whereParams := map[string]any{"app_type": appType}

	if clearEntity != "" {
		whereClause += " AND applications.entity_type = @entity_type"
		whereParams["entity_type"] = clearEntity
	}

	if queryParams.Applicant != "" {
		whereClause += " AND applications.applicant = @applicant"
		whereParams["applicant"] = queryParams.Applicant
	}

	if queryParams.State != "" {
		if !lo.Contains([]string{"open", "approved", "rejected", "processing", "completed"}, clearState) {
			return nil, 0, fmt.Errorf("unknown state %s", queryParams.State)
		}
		whereClause += " AND applications.state = @state"
		whereParams["state"] = ApplicationState(clearState)
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

		whereClause += " AND applications.created_at >= @start_date AND applications.created_at <= @end_date"
		whereParams["start_date"] = startDate
		whereParams["end_date"] = endDate
	}

	if len(strings.TrimSpace(queryParams.EntityId)) != 0 {
		whereClause += " AND applications.entity_id = @entity_id"
		whereParams["entity_id"] = strings.TrimSpace(queryParams.EntityId)
	}

	if queryParams.SortField == "" {
		queryParams.SortField = "created_at"
	}

	if queryParams.Size == 0 {
		queryParams.Size = api.DefaultPageSize
	}

	if queryParams.Page == 0 {
		queryParams.Page = 1
	}

	if queryParams.SortOrder == "" {
		queryParams.SortOrder = "desc"
	}

	// TODO: Currently the key for detailed data is budget type, which will be changed to asset name in future, and this query should also be updated
	//if queryParams.UserWallet != "" {
	//	querySeg = querySeg.
	//		Where(datatypes.JSONQuery("detailed_data").Equals(queryParams.UserWallet, "credit", "user_wallet")).
	//		Or(datatypes.JSONQuery("detailed_data").Equals(queryParams.UserWallet, "token", "user_wallet"))
	//}

	// TODO: This is the mysql style, need to find way to get db schema here and implement pg way
	whereClause += "\nORDER BY @order_by LIMIT @offset, @limit"
	whereParams["order_by"] = fmt.Sprintf("%s %s", queryParams.SortField, queryParams.SortOrder)
	whereParams["offset"] = (queryParams.Page - 1) * queryParams.Size
	whereParams["limit"] = queryParams.Size

	fmt.Printf("TTT: query sql: %s\n", querySQL+whereClause)
	fmt.Printf("TTT: where params: %+v\n", whereParams)

	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return tx.Raw(querySQL+whereClause, whereParams)
	})
	fmt.Printf("TTT: sql: %+s\n", sql)

	var foobar []map[string]any
	err := db.Raw(querySQL+whereClause, whereParams).Find(&foobar).Error
	if err != nil {
		panic(err)
	}
	for _, r := range foobar {
		fmt.Printf("TTT: Rcd: %+v\n", r)
	}

	//total, err := gormfind.Count(querySeg)
	//if err != nil {
	//	return nil, 0, err
	//}
	//
	//rcds, err := gormfind.RowsJoin[jointAppProjectRslt](querySeg, "applications", &gormFindPage)
	//if err != nil {
	//	return nil, 0, err
	//}
	//
	//rslt := make([]*FrontendApplicationRecord, len(rcds))
	//
	//for i, r := range rcds {
	//	rslt[i] = r.ToFrontedApplicationRecord(db)
	//}

	return nil, 0, nil
}
