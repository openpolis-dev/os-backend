package model

// Application model saves all applications made by user, which includes close_project
// and new_reward currently.

import (
	"fmt"
	"strings"
	"time"

	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type Application struct {
	// unique ID for this request
	ID uint `json:"id" gorm:"primaryKey"`

	// DisplayGroupId is used to group records should be displayed in one line in frontend page
	DisplayGroupId string `json:"display_group_id"`

	// application type
	Type ApplicationType `json:"type"`

	// Member send this application
	Applicant string `json:"applicant"`

	// Application state, which contains open/approved/rejected/processing/completed
	State ApplicationState `json:"state"`

	// Saves the reject reason if this application state is rejected
	RejectReason string `json:"reject_reason"`

	// CompleteMessage saves
	CompleteMessage string `json:"complete_message"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Detailed data saves application specified data
	// Currently the design is using this struct for all types of application, if new fields are required for new type
	// of application, just add the field in the struct and let code branch choose which fields are required
	DetailedData ApplicationDetailedData `json:"detailed_data,omitempty" gorm:"foreignKey:ApplicationID;references:ID"`

	// Entity means this application's refer, which maybe project or guild.
	// And the field EntityId is the db record ID for Project or Guild table
	EntityType string `json:"entity_type"`
	EntityId   uint   `json:"entity_id"`
}

type ApplicationDetailedData struct {
	ApplicationID uint `json:"application_id"`

	// TargetUserWallet saves user wallet address that the reward will be sent to
	TargetUserWallet string `json:"user_wallet"`

	// AssetName and Amount saves the token related info about this reward application
	AssetName string `json:"asset_name"`
	Amount    uint64 `json:"amount"`
}

type ApplicationAuditLog struct {
	// unique ID of this audit log
	ID uint `json:"id" gorm:"primaryKey"`

	// Which application this audit log belongs to
	ApplicationID uint `json:"application_id"`

	LogTs time.Time `json:"log_ts"`

	// Which operation this log record, which should be in new/approve/reject/process/complete
	Operation AuditActionType `json:"operation"`

	// Who perform this operation
	Operator string `json:"operator"`

	// Application state before this operation
	PreState ApplicationState `json:"pre_state"`

	// Application state after this operation
	PostState ApplicationState `json:"post_state"`

	// ExtraData saves some additional data for the operation, e.g. reject reason
	ExtraData string `json:"extra_data"`
}

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

// ValidateAuditAction validates whether the action required is suit for current application state.
// Returns true means the action can be applied to application, while false means the action is invalid
func (app *Application) ValidateAuditAction(action AuditActionType) bool {
	if actions, foundState := applicationStateMap[app.State]; foundState {
		_, foundAction := actions[action]
		return foundAction
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
func AuditApplication(db *gorm.DB, operatorWallet string, application *Application, action AuditActionType, extraMsg string) error {
	// Check application record, verify whether the action can be applied on the application
	if !application.ValidateAuditAction(action) {
		// TODO: Define the error message as project constant
		return fmt.Errorf("application state %s is not suite for action %s", application.State, action)
	}

	// Check related entity record, verify whether the status of entity is same with application post_state
	if application.EntityType == "project" {
		project, err := ProjectModel.Detail(db, application.EntityId)
		if err != nil {
			return err
		}
		// For close_project application, the project should be in pending_close state
		// For new_reward application, the project should be in open state
		if (application.Type == ApplicationCloseProject && project.Status != ProjectStatusPendingClose) ||
			(application.Type == ApplicationNewReward && project.Status != ProjectStatusOpen) {
			return fmt.Errorf("can not apply application type %s on project with status %s", application.Type, project.Status)
		}
	}

	err := userWalletRecordExisting(db, operatorWallet)
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		return doAuditApplicationInTransaction(tx, operatorWallet, application, action, extraMsg)
	})
}

// BatchAuditApplication audits multiple applications in same transaction.
// Note: if any error occurred during the transaction the whole transaction will not be performed.
func BatchAuditApplication(db *gorm.DB, operatorWallet string, applications *[]Application, action AuditActionType, extraMsg string) error {
	for _, application := range *applications {
		if !application.ValidateAuditAction(action) {
			// TODO: Define the error message as project constant
			return fmt.Errorf("application state %s is not suite for action %s", application.State, action)
		}
	}

	err := userWalletRecordExisting(db, operatorWallet)
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, application := range *applications {
			err = doAuditApplicationInTransaction(tx, operatorWallet, &application, action, extraMsg)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func doAuditApplicationInTransaction(tx *gorm.DB, operatorWallet string, application *Application, action AuditActionType, extraMsg string) error {
	nextState := application.nextStateAfterAction(action)

	// Create audit log for application
	if err := tx.Create(&ApplicationAuditLog{
		ApplicationID: application.ID,
		LogTs:         time.Now(),
		Operation:     action,
		Operator:      operatorWallet,
		PreState:      application.State,
		PostState:     nextState,
		ExtraData:     extraMsg,
	}).Error; err != nil {
		return err
	}

	// Update application state and additional message
	application.State = nextState
	if action == AuditActionReject {
		application.RejectReason = extraMsg
	} else if action == AuditActionComplete {
		application.CompleteMessage = extraMsg
	}

	if err := tx.Save(&application).Error; err != nil {
		return err
	}

	if nextState == ApplicationStateProcessing {
		if application.Type == ApplicationNewReward {
			// The NewReward application getting into processing state requires some updates on assets of project and user
			// * For project/guild, find budget record and extract amount from remainAmount
			// * For user, update asset record with asset name and processing amount.
			if application.EntityType == "project" {
				// Update project budget
				if err := ProjectModel.WithdrawBudget(tx, application.EntityId, application.DetailedData.AssetName, application.DetailedData.Amount); err != nil {
					return err
				}

				// Update user asset record
				if err := UserAssetRecordModel.CreateOrUpdate(tx, application.Applicant, application.DetailedData.AssetName, application.DetailedData.Amount, 0); err != nil {
					return err
				}
			} else if application.EntityType == "guild" {
				// TODO: Guild is not implemented yet
			} else {
				return fmt.Errorf("unknown application entity type %s", application.EntityType)
			}
		}
	} else if nextState == ApplicationStateRejected {
		if application.Type == ApplicationNewReward {
			// Application has been rejected, if it is new reward request, add budget back to project/guild and remove user processing asset amount
			if err := ProjectModel.DepositBudget(tx, application.EntityId, application.DetailedData.AssetName, application.DetailedData.Amount); err != nil {
				return err
			}

			// Update user asset record
			if err := UserAssetRecordModel.Rollback(tx, application.Applicant, application.DetailedData.AssetName, application.DetailedData.Amount, 0); err != nil {
				return err
			}
		}
	} else if nextState == ApplicationStateCompleted {
		if application.Type == ApplicationCloseProject {
			// This is a close project application, so the `entity_id` saved indicates a project record
			project, err := ProjectModel.Detail(tx, application.EntityId)
			if err != nil {
				return err
			}
			project.Status = ProjectStatusClosed
			return tx.Save(project).Error
		} else if application.Type == ApplicationNewReward {
			// For new reward application, the `entity_type` is required to get related db table
			// The main steps for the post complete operation are:
			// * Add the amount to target user
			// Update user asset record
			if err := UserAssetRecordModel.CompleteAssetTransaction(tx, application.Applicant, application.DetailedData.AssetName, application.DetailedData.Amount); err != nil {
				return err
			}
		}
	}

	return nil
}

func userWalletRecordExisting(db *gorm.DB, walletAddr string) error {
	userCnt := int64(0)
	err := db.Model(&User{}).Where("wallet = ?", strings.ToLower(walletAddr)).Count(&userCnt).Error
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

func (app *Application) ListAuditLogs(db *gorm.DB) ([]*ApplicationAuditLog, error) {
	querySeg := db.Model(&ApplicationAuditLog{}).Where("application_id = ?", app.ID)
	return gormfind.Rows[ApplicationAuditLog](querySeg, nil)
}
