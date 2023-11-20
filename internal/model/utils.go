package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"gorm.io/gorm"
)

const DateQueryFormat = "2006-01-02"

const QueryApplicationsWithEntityNameBaseSQL = `
SELECT app.id                 as application_id,
       seasons.name           as season_name,
       CASE
           WHEN app.entity_type = 'project' THEN projects.name
           WHEN app.entity_type = 'guild' THEN guilds.name
           ELSE NULL END      AS entity_name,
       CASE
           WHEN app.entity_type = 'project' THEN projects.name
           WHEN app.entity_type = 'guild' THEN guilds.name
           ELSE NULL END      AS budget_source,
       apply_aal.operator     as applicant_wallet,
       apply_aal.log_ts       as apply_ts,
       applicant.avatar       as applicant_avatar,

       review_aal.operator    as reviewer_wallet,
       review_aal.log_ts      as review_ts,
       reviewer.avatar        as reviewer_avatar,

       process_aal.operator   as processor_wallet,
       process_aal.log_ts     as process_ts,
       processor.avatar       as processor_avatar,

       completed_aal.operator as completer_wallet,
       completed_aal.log_ts   as complete_ts,
       completer.avatar       as completer_avatar,

       app.created_at         as created_at,
       app.create_ts          as create_ts,
       app.update_ts          as update_ts,
       app.comment            as comment,
       app.target_user_wallet,
       app.asset_name,
       app.asset_amount       as amount,
       app.state              as status,
       app.detailed_type,
       app.comment,
       completed_aal.operator as reviewer_wallet,
       app.complete_message   as transaction_ids,
       app_bundles.comment as app_bundle_comment
FROM applications as app
         LEFT JOIN projects ON app.entity_type = 'project' AND app.entity_id = projects.id
         LEFT JOIN guilds ON app.entity_type = 'guild' AND app.entity_id = guilds.id
         LEFT JOIN seasons ON app.season_id = seasons.id
         LEFT JOIN application_audit_logs completed_aal on app.id = completed_aal.application_id AND completed_aal.id =
                                                                                                     (select max(id)
                                                                                                      from application_audit_logs aal
                                                                                                      where aal.application_id = app.id
                                                                                                        and aal.post_state = 'completed')
         LEFT JOIN application_audit_logs process_aal on app.id = process_aal.application_id AND process_aal.id =
                                                                                                 (select max(id)
                                                                                                  from application_audit_logs aal
                                                                                                  where aal.application_id = app.id
                                                                                                    and aal.post_state = 'processing')
         LEFT JOIN application_audit_logs review_aal on app.id = review_aal.application_id AND review_aal.id =
                                                                                               (select max(id)
                                                                                                from application_audit_logs aal
                                                                                                where aal.application_id = app.id
                                                                                                  and aal.post_state = 'approved')
         LEFT JOIN application_audit_logs apply_aal
                   on app.id = apply_aal.application_id AND apply_aal.id = (select max(id)
                                                                            from application_audit_logs aal
                                                                            where aal.application_id = app.id
                                                                              and aal.post_state = 'open')
         LEFT JOIN users applicant ON applicant.wallet = apply_aal.operator
         LEFT JOIN users reviewer ON reviewer.wallet = review_aal.operator
         LEFT JOIN users processor ON processor.wallet = process_aal.operator
         LEFT JOIN users completer ON completer.wallet = completed_aal.operator
         LEFT JOIN app_bundles ON app.bundle_id = app_bundles.id`

const QueryAppBundlesWithEntityNameBaseSQL = `SELECT app_bundles.*,
CASE
   WHEN app_bundles.entity_type = 'project' THEN projects.name
   WHEN app_bundles.entity_type = 'guild' THEN guilds.name
   ELSE NULL END AS entity_name,
seasons.name AS season_name
FROM app_bundles
   LEFT JOIN seasons ON app_bundles.season_id = seasons.id
   LEFT JOIN projects ON app_bundles.entity_type = 'project' AND app_bundles.entity_id = projects.id
   LEFT JOIN guilds ON app_bundles.entity_type = 'guild' AND app_bundles.entity_id = guilds.id`

var CloseProjectStateOrder = []string{string(ApplicationStateOpen), ApplicationStateCompleted, ApplicationStateRejected}

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
			LogTs:         GetCurrentUtcEpochSecond(),
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
	querySQL := QueryApplicationsWithEntityNameBaseSQL + " WHERE applications.id IN ?"

	var projectRcds []jointAppEntityRslt
	err := db.Raw(querySQL, ids).Find(&projectRcds).Error
	if err != nil {
		log.Error().Msgf("query joint app entity error: %+v", err)
		return nil, err
	}

	rslt := make([]*FrontendApplicationRecord, len(projectRcds))
	for i, r := range projectRcds {
		rslt[i] = r.ToFrontedApplicationRecord(db)
	}

	return rslt, nil
}

// GenerateFrontendApplicationRecords filter application records from DB with params and convert to predefined format used for frontend page
// TODO: Check whether some generic function can be used to merge duplicated logic in this function and QueryAppBundleRecords
func GenerateFrontendApplicationRecords(db *gorm.DB, queryParams *ListApplicationQueryParams) ([]*FrontendApplicationRecord, int64, error) {
	clearAppType := strings.ToLower(strings.TrimSpace(queryParams.Type))
	clearEntity := strings.ToLower(strings.TrimSpace(queryParams.Entity))
	clearState := strings.ToLower(strings.TrimSpace(queryParams.State))
	clearAssetName := strings.ToLower(strings.TrimSpace(queryParams.AssetName))

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
	whereClause := "\nWHERE app.type = @app_type"
	whereParams := map[string]any{"app_type": appType}

	if clearEntity != "" {
		whereClause += " AND app.entity_type = @entity_type"
		whereParams["entity_type"] = clearEntity
	}

	if clearAssetName != "" {
		whereClause += " AND LOWER(applications.asset_name) = @asset_name"
		whereParams["asset_name"] = clearAssetName
	}

	if queryParams.Applicant != "" {
		whereClause += " AND app.applicant = @applicant"
		whereParams["applicant"] = queryParams.Applicant
	}

	if queryParams.State != "" {
		if !lo.Contains([]string{"open", "approved", "rejected", "processing", "completed"}, clearState) {
			return nil, 0, fmt.Errorf("unknown state %s", queryParams.State)
		}
		whereClause += " AND app.state = @state"
		whereParams["state"] = ApplicationState(clearState)
	}

	if queryParams.StartDate != "" && queryParams.EndDate != "" {
		startDate, err := time.Parse(DateQueryFormat, queryParams.StartDate)
		if err != nil {
			return nil, 0, err
		}

		endDate, err := time.Parse(DateQueryFormat, queryParams.EndDate)
		if err != nil {
			return nil, 0, err
		}

		whereClause += " AND app.created_at >= @start_date AND app.created_at <= @end_date"
		whereParams["start_date"] = startDate
		whereParams["end_date"] = endDate
	}

	if len(strings.TrimSpace(queryParams.EntityId)) != 0 {
		whereClause += " AND app.entity_id = @entity_id"
		whereParams["entity_id"] = strings.TrimSpace(queryParams.EntityId)
	}

	if queryParams.Size == 0 {
		queryParams.Size = internal.DefaultPageSize
	}

	if queryParams.Page == 0 {
		queryParams.Page = 1
	}

	orderByClause := ""
	if clearAppType == "close_project" {
		// For close_project type, the return records should have stable order
		querySort := lo.Map(CloseProjectStateOrder, func(state string, index int) string {
			return fmt.Sprintf("when app.state='%s' then %d\n", state, index+1)
		})
		orderByClause = fmt.Sprintf("case \n%s end asc, app.created_at desc", strings.Join(querySort, ""))
	} else {
		if queryParams.SortField == "" {
			queryParams.SortField = "created_at"
		}

		if queryParams.SortOrder == "" {
			queryParams.SortOrder = "desc"
		}

		orderByClause = fmt.Sprintf("app.%s %s ", queryParams.SortField, queryParams.SortOrder)
	}

	if queryParams.UserWallet != "" {
		whereClause += " AND app.target_user_wallet = @target_user_wallet"
		whereParams["target_user_wallet"] = strings.ToLower(strings.TrimSpace(queryParams.UserWallet))
	}

	if queryParams.SeasonId != 0 {
		whereClause += " AND app.season_id = @season_id"
		whereParams["season_id"] = queryParams.SeasonId
	}

	// Calculate total count
	total := db.Raw(querySQL+whereClause, whereParams).Scan(&[]map[string]any{}).RowsAffected

	// TODO: This is the mysql style, need to find way to get db schema here and implement pg way
	whereClause += fmt.Sprintf("\nORDER BY %s LIMIT @offset, @limit", orderByClause)
	whereParams["offset"] = (queryParams.Page - 1) * queryParams.Size
	whereParams["limit"] = queryParams.Size

	var rslt []*FrontendApplicationRecord
	err := db.Raw(querySQL+whereClause, whereParams).Find(&rslt).Error
	if err != nil {
		log.Error().Msgf("get application list error: %+v, query sql: %s, query params: %+v", err, querySQL+whereClause, whereParams)
		return nil, 0, err
	}

	return rslt, total, nil
}

func QueryAppBundleRecords(db *gorm.DB, queryParams *ListAppBundleQueryParams) ([]JointAppBundleEntityRslt, int64, error) {
	clearedEntity := strings.ToLower(strings.TrimSpace(queryParams.Entity))
	clearState := strings.ToLower(strings.TrimSpace(queryParams.State))

	if clearedEntity != "" {
		if !lo.Contains([]string{"project", "guild"}, clearedEntity) {
			return nil, 0, fmt.Errorf("unknown entity type %s", queryParams.Entity)
		}
	}

	querySQL := QueryAppBundlesWithEntityNameBaseSQL
	whereClause := "\nHAVING app_bundles.shadow_record=false and type=@type"
	whereParams := map[string]any{
		"type": "NEW_REWARD",
	}

	// TODO: Dup logic start
	if clearState != "" {
		if !lo.Contains([]string{"open", "approved", "rejected"}, clearState) {
			return nil, 0, fmt.Errorf("unknown state %s", queryParams.State)
		}
		whereClause += " AND app_bundles.state = @state"
		whereParams["state"] = ApplicationState(clearState)
	}

	if clearedEntity != "" {
		whereClause += " AND app_bundles.entity_type = @entity_type"
		whereParams["entity_type"] = clearedEntity
	}

	if queryParams.Applicant != "" {
		whereClause += " AND app_bundles.applicant = @applicant"
		whereParams["applicant"] = queryParams.Applicant
	}

	if len(strings.TrimSpace(queryParams.EntityId)) != 0 {
		whereClause += " AND app_bundles.entity_id = @entity_id"
		whereParams["entity_id"] = strings.TrimSpace(queryParams.EntityId)
	}

	if queryParams.SortField == "" {
		queryParams.SortField = "created_at"
	}

	if queryParams.Size == 0 {
		queryParams.Size = internal.DefaultPageSize
	}

	if queryParams.Page == 0 {
		queryParams.Page = 1
	}

	if queryParams.SortOrder == "" {
		queryParams.SortOrder = "desc"
	}

	if queryParams.SeasonId != 0 {
		whereClause += " AND app_bundles.season_id = @season_id"
		whereParams["season_id"] = queryParams.SeasonId
	}
	// TODO: Dup logic end

	// Calculate total count
	total := db.Raw(querySQL+whereClause, whereParams).Scan(&[]map[string]any{}).RowsAffected

	// TODO: This is the mysql style, need to find way to get db schema here and implement pg way
	whereClause += fmt.Sprintf("\nORDER BY app_bundles.%s %s LIMIT @offset, @limit", queryParams.SortField, queryParams.SortOrder)
	whereParams["offset"] = (queryParams.Page - 1) * queryParams.Size
	whereParams["limit"] = queryParams.Size

	var rcds []JointAppBundleEntityRslt
	err := db.Raw(querySQL+whereClause, whereParams).Find(&rcds).Error
	if err != nil {
		return nil, 0, err
	}

	return rcds, total, nil
}

func ConvertTimeToTzString(t time.Time, timeLoc string, timeFormat string) (string, error) {
	loc, err := time.LoadLocation(timeLoc)
	if err != nil {
		return "", err
	}
	locTime := t.In(loc) // convert to UTC+8 timezone
	return locTime.Format(timeFormat), nil
}

func SetDefaultMapValue[K comparable, V any](origMap map[K]V, key K, value V) {
	_, ok := origMap[key]
	if !ok {
		origMap[key] = value
	}
}
func GetMapValueOrDefault[K comparable, V any](origMap map[K]V, key K, defaultValue V) V {
	_, ok := origMap[key]
	if !ok {
		return defaultValue
	} else {
		return origMap[key]
	}
}

func FormatUserWallet(wallet string) string {
	return strings.TrimSpace(strings.ToLower(wallet))
}

func GetCurrentUtcEpochSecond() int64 {
	return time.Now().UTC().Unix()
}
