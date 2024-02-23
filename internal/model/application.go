package model

// Application model saves all applications made by user, which includes close_project
// and new_reward currently.

import (
	"fmt"
	"strings"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var err error

type Application struct {
	// unique ID for this request
	ID uint `json:"id" gorm:"primaryKey"`

	// application type
	Type ApplicationType `json:"type" gorm:"index"`

	// SubType saves an optional type for the application.
	// And the data currently is only used by backend code, no frontend logic should relay on this
	SubType string `json:"sub_type" gorm:"index"`

	// Member send this application
	Applicant string `json:"applicant"`

	// Application state, which contains open/approved/rejected/processing/completed
	State ApplicationState `json:"state" gorm:"index"`

	// Saves the reject reason if this application state is rejected
	RejectReason string `json:"reject_reason"`

	// CompleteMessage saves
	CompleteMessage string `json:"complete_message"`

	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`

	CreateTs int64 `json:"create_ts" gorm:"index"`
	UpdateTs int64 `json:"update_ts" gorm:"index"`

	// DetailedType means a sub category of this application
	// Value for this field saves data from Excel data
	DetailedType string `json:"detailed_type"`

	// Comment saves some user entered data
	Comment string `json:"comment"`

	// DetailedData saves application detailed data
	// Currently the design is using this struct to save serialized detailed data for all applications.
	// The data will be deserialized to specified struct before using
	DetailedData datatypes.JSON `json:"detailed_data,omitempty"`

	// In OS ver 2.0, a single application should only contain single asset record.
	// The DetailedData field will be abandoned after v2.0
	AssetName        string          `json:"asset_name" gorm:"index"`
	AssetAmount      decimal.Decimal `json:"asset_amount" sql:"type:decimal(20,8);"`
	TargetUserWallet string          `json:"target_user_wallet" gorm:"index"`

	// Entity means this application's refer, which maybe project or guild.
	// And the field EntityId is the db record ID for Project or Guild table
	EntityType string `json:"entity_type" gorm:"index"`
	EntityId   uint   `json:"entity_id" gorm:"index"`

	// Season information of application
	SeasonId uint    `json:"season_id" gorm:"index"`
	Season   *Season `json:"season"`

	BundleId uint `json:"bundle_id"`
}

type ApplicationAuditLog struct {
	// unique ID of this audit log
	ID uint `json:"id" gorm:"primaryKey"`

	// Which application this audit log belongs to
	ApplicationID uint `json:"application_id" gorm:"index"`

	LogTs int64 `json:"log_ts" gorm:"index"`

	// Which operation this log record, which should be in new/approve/reject/process/complete
	Operation AuditActionType `json:"operation"`

	// Who perform this operation
	Operator string `json:"operator" gorm:"index"`

	// Application state before this operation
	PreState ApplicationState `json:"pre_state"`

	// Application state after this operation
	PostState ApplicationState `json:"post_state" gorm:"index"`

	// ExtraData saves some additional data for the operation, e.g. reject reason
	ExtraData string `json:"extra_data"`
}

// ValidateAuditAction validates whether the action required is suit for current application state.
// Returns true means the action can be applied to application, while false means the action is invalid
func (app *Application) ValidateAuditAction(db *gorm.DB, action AuditActionType) bool {
	db.Find(&app, app.ID)
	if err != nil {
		log.Error().Msgf("Get application %d state error: %+v", app.ID, err)
		return false
	}
	if actions, stateFoundFlag := applicationStateMap[app.State]; stateFoundFlag {
		_, actionFoundFlag := actions[action]
		return actionFoundFlag
	} else {
		return false
	}
}

// nextStaterAfterAction returns next state if applying action to current application
func (app *Application) nextStateAfterAction(action AuditActionType) ApplicationState {
	actions, _ := applicationStateMap[app.State]
	nextState, _ := actions[action]

	// For close project request, approved means project can be closed, and the project will be closed automatically,
	// no further state are required
	if app.Type == ApplicationCloseProject && nextState == ApplicationStateApproved {
		nextState = ApplicationStateCompleted
	}
	return nextState
}

// AuditApplication applies audit action on application and create related audit log in transaction
func AuditApplication(db *gorm.DB, operatorWallet string, application *Application, action AuditActionType, extraMsg string, enforcer *casbin.SyncedEnforcer, push []sdk.Pusher) error {
	// Check application record, verify whether the action can be applied on the application
	if !application.ValidateAuditAction(db, action) {
		return fmt.Errorf("application state %s is not suite for action %s", application.State, action)
	}

	// Check related entity record, verify whether the status of entity is same with application post_state
	if application.EntityType == "project" {
		project, err := ProjectModel.Detail(db, application.EntityId)
		if err != nil {
			log.Error().Msgf("Fetch project %d error: %+v", application.EntityId, err)
			return err
		}

		if project == nil {
			err := fmt.Errorf("project %d not found", application.EntityId)
			log.Error().Msgf("Fetch project %d error: %+v", application.EntityId, err)
			return err
		}
	}

	err := userWalletRecordExisting(db, operatorWallet)
	if err != nil {
		log.Error().Msgf("Check user wallet error: %+v", err)
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		return doAuditApplicationInTransaction(tx, operatorWallet, application, action, extraMsg, enforcer, push)
	})
}

// BatchAuditApplication audits multiple applications in same transaction.
// Note: if any error occurred during the transaction the whole transaction will not be performed.
// This function gets applications object reference in params, which should be changed to pass by id, but the update for invoker may bring other changes, so the solution is adding some refresh of object in the code.
func BatchAuditApplication(db *gorm.DB, operatorWallet string, applications *[]Application, action AuditActionType, extraMsg string, enforcer *casbin.SyncedEnforcer, push []sdk.Pusher) error {
	err := userWalletRecordExisting(db, operatorWallet)
	if err != nil {
		log.Error().Msgf("Check user wallet error: %+v", err)
		return err
	}

	// Validate application state
	for _, application := range *applications {
		if !application.ValidateAuditAction(db, action) {
			return fmt.Errorf("application state %s is not suite for action %s", application.State, action)
		}
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, application := range *applications {
			err = doAuditApplicationInTransaction(tx, operatorWallet, &application, action, extraMsg, enforcer, push)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func doAuditApplicationInTransaction(tx *gorm.DB, operatorWallet string, application *Application, action AuditActionType, extraMsg string, enforcer *casbin.SyncedEnforcer, push []sdk.Pusher) error {
	// Refresh application record
	tx.Find(&application, application.ID)
	nextState := application.nextStateAfterAction(action)

	// Create audit log for application
	if err := tx.Create(&ApplicationAuditLog{
		ApplicationID: application.ID,
		LogTs:         GetCurrentUtcEpochSecond(),
		Operation:     action,
		Operator:      common.FormatUserWallet(operatorWallet),
		PreState:      application.State,
		PostState:     nextState,
		ExtraData:     extraMsg,
	}).Error; err != nil {
		log.Error().Msgf("Create application audit log error: %+v", err)
		return err
	}

	// Update application state and additional message
	application.State = nextState
	if action == AuditActionReject {
		application.RejectReason = extraMsg
		if application.Type == ApplicationCloseProject {
			if err := ProjectModel.UpdateProjectStatus(tx, application.EntityId, ProjectStatusOpen); err != nil {
				log.Error().Msgf("update project %d status back to open error: %+v", application.EntityId, err)
				return err
			}
		}
	} else if action == AuditActionComplete {
		application.CompleteMessage = extraMsg
	}

	application.UpdatedAt = time.Now().In(internal.ProjectTimezone)
	application.UpdateTs = GetCurrentUtcEpochSecond()
	if err := tx.Updates(&application).Error; err != nil {
		log.Error().Msgf("Update application error: %+v", err)
		return err
	}

	if nextState == ApplicationStateProcessing {
		err := processingApplication(tx, application)
		if err != nil {
			log.Error().Msgf("processing application error: %+v", err)
			return err
		}
	} else if nextState == ApplicationStateCompleted {
		err := completeApplication(tx, operatorWallet, application, enforcer, push)
		if err != nil {
			log.Error().Msgf("complete application error: %+v", err)
			return err
		}
	}

	return nil
}

func processingApplication(tx *gorm.DB, application *Application) error {
	if application.Type == ApplicationNewReward {
		// TODO: UserAssetRecord is not using for now, need to confirm how to handle this in the future
		// Changes in OS ver 2.0
		// * No budget for project and guild
		// * One application only saves one type of asset, and the fields are extracted from DetailedData
		// Update user asset record
		//if err := UserAssetRecordModel.CreateOrUpdate(tx, application.TargetUserWallet, application.AssetName, application.AssetAmount, decimal.Zero); err != nil {
		//	return err
		//}
	}
	return nil
}

// completeApplication performs the associated operations and marks the application to complete state
func completeApplication(tx *gorm.DB, operatorWallet string, application *Application, enforcer *casbin.SyncedEnforcer, push []sdk.Pusher) error {
	if application.Type == ApplicationCloseProject {
		// This is a close project application, so the `entity_id` saved indicates a project record
		if err := ProjectModel.UpdateProjectStatus(tx, application.EntityId, ProjectStatusClosed); err != nil {
			log.Error().Msgf("update project %d status to closed error: %+v", application.EntityId, err)
			return err
		}

		project, err := ProjectModel.Detail(tx, application.EntityId)
		if err != nil {
			log.Error().Msgf("Fetch project %d error: %+v", application.EntityId, err)
			return err
		}

		// TODO: Verify whether those duplicated code can be merged into some functions
		// clean rbac if passed in enforcer
		if enforcer != nil {
			// remove policies
			policies := GenerateCasbinPolicies(application.EntityId)
			if _, err := enforcer.RemovePolicies(policies); err != nil {
				log.Error().Msgf("remove casbin policies error: %+v", err)
				return err
			}
			// remove roles for sponsors
			oldSponsorGroupingPolicies := lo.Map(project.Sponsors, func(sponsor string, _ int) []string {
				// g, 0xc13..1283 proj_sponsor_1
				return []string{sponsor, fmt.Sprintf("%s%d", internal.RoleProjSponsorPrefix, project.ID)}
			})
			_, err = enforcer.RemoveGroupingPolicies(oldSponsorGroupingPolicies)
			if err != nil {
				return err
			}
			err = enforcer.SavePolicy()
			if err != nil {
				return err
			}
			//// remove roles for members
			//oldMemberGroupingPolicies := lo.Map(project.Members, func(member string, _ int) []string {
			//	// g, 0xc13..1283 proj_member_1
			//	return []string{member, fmt.Sprintf("%s%d", api.RoleProjMemberPrefix, project.ID)}
			//})
			//_, err = enforcer.RemoveGroupingPolicies(oldMemberGroupingPolicies)
			//if err != nil {
			//	return err
			//}
		}
	} else if application.Type == ApplicationNewReward {
		// TODO: UserAssetRecord is not using for now, need to confirm how to handle this in the future
		// For new reward application, the `entity_type` is required to get related db table
		// The main steps for the post complete operation are:
		// * Add the amount to target user

		//if err := UserAssetRecordModel.CompleteAssetTransaction(tx, application.TargetUserWallet, application.AssetName, application.AssetAmount); err != nil {
		//	return err
		//}
	}
	return nil
}

func (app *Application) ListAuditLogs(db *gorm.DB) ([]*ApplicationAuditLog, error) {
	querySeg := db.Model(&ApplicationAuditLog{}).Where("application_id = ?", app.ID)
	return QueryRows[ApplicationAuditLog](querySeg, nil)
}

func (app *Application) GetLatestAuditLog(db *gorm.DB) (*ApplicationAuditLog, error) {
	querySeg := db.Model(&ApplicationAuditLog{}).Where("application_id = ?", app.ID).Order("log_ts desc").Order("id desc")
	return gormfind.Row[ApplicationAuditLog](querySeg)
}

func userWalletRecordExisting(db *gorm.DB, walletAddr string) error {
	userCnt := int64(0)
	err := db.Model(&User{}).Where("wallet = ?", common.FormatUserWallet(walletAddr)).Count(&userCnt).Error
	if err != nil {
		return err
	}

	if userCnt == 0 {
		return fmt.Errorf("wallet record %s is not existing", walletAddr)
	} else if userCnt > 1 {
		return fmt.Errorf("wallet %s has more than one record, contract admin to fix this", walletAddr)
	}

	return nil
}

// GetCurrentSeasonApplications returns application in current season with state and type filter passed in
func GetCurrentSeasonApplications(db *gorm.DB, states []string, appTypes []string) ([]*Application, error) {
	currentSeason, err := GetCurrentSeason(db)
	if err != nil {
		log.Error().Msgf("get current season error: %+v", err)
		return nil, err
	}

	whereClause := "season_id = @season_id"
	whereParams := map[string]any{"season_id": currentSeason.ID}

	if len(states) > 0 {
		whereClause += " AND state IN ('" + strings.Join(states, "','") + "')"
	}

	if len(appTypes) > 0 {
		whereClause += " AND type IN ('" + strings.Join(appTypes, "','") + "')"
	}

	var records []*Application
	err = db.Model(Application{}).Where(whereClause, whereParams).Find(&records).Error

	if err != nil {
		log.Error().Msgf("query current season applications error: %+v", err)
		return nil, err
	}

	return records, nil
}
