package model

// Application model saves all applications made by user, which includes close_project
// and new_reward currently.

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type Application struct {
	// unique ID for this request
	ID uint `json:"id" gorm:"primaryKey"`

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

	// DetailedType means a sub category of this application
	DetailedType string `json:"detailed_type"`

	// Comment saves some user entered data
	Comment string `json:"comment"`

	// DetailedData saves application detailed data
	// Currently the design is using this struct to save serialized detailed data for all applications.
	// The data will be deserialized to specified struct before using
	DetailedData []byte `json:"detailed_data,omitempty"`

	// Entity means this application's refer, which maybe project or guild.
	// And the field EntityId is the db record ID for Project or Guild table
	EntityType string `json:"entity_type"`
	EntityId   uint   `json:"entity_id"`
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
		if application.Type == ApplicationCloseProject {
			project, err := ProjectModel.Detail(tx, application.EntityId)
			if err != nil {
				return err
			}
			project.Status = ProjectStatusOpen
			err = tx.Save(project).Error
			if err != nil {
				return err
			}
		}
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
				detailedData := NewRewardApplicationDetailedData{}
				err := json.Unmarshal(application.DetailedData, &detailedData)
				if err != nil {
					return err
				}

				for budgetType, detail := range detailedData {
					// Update project budget
					if err := ProjectModel.WithdrawBudget(tx, application.EntityId, budgetType, detail.AssetName, detail.Amount); err != nil {
						return err
					}

					// Update user asset record
					if err := UserAssetRecordModel.CreateOrUpdate(tx, application.Applicant, budgetType, detail.Amount, 0); err != nil {
						return err
					}
				}
			} else if application.EntityType == "guild" {
				detailedData := NewRewardApplicationDetailedData{}
				err := json.Unmarshal(application.DetailedData, &detailedData)
				if err != nil {
					return err
				}

				for budgetType, detail := range detailedData {
					// Update guild budget
					if err := GuildModel.WithdrawBudget(tx, application.EntityId, budgetType, detail.AssetName, detail.Amount); err != nil {
						return err
					}

					// Update user asset record
					if err := UserAssetRecordModel.CreateOrUpdate(tx, application.Applicant, budgetType, detail.Amount, 0); err != nil {
						return err
					}
				}
			} else {
				return fmt.Errorf("unknown application entity type %s", application.EntityType)
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

			// Deposit project remain budget back to treasure
			budgets, err := ProjectBudgetModel.ListByProjectId(tx, project.ID)
			if err != nil {
				return err
			}
			for _, budget := range budgets {
				err = TreasuryAssetHelper.DepositTreasureAsset(tx, budget.Type, budget.Name, budget.TotalAmount, operatorWallet, fmt.Sprintf("Close project %d by %s", project.ID, operatorWallet))
				if err != nil {
					return err
				}
			}

			return tx.Save(project).Error
		} else if application.Type == ApplicationNewReward {
			// For new reward application, the `entity_type` is required to get related db table
			// The main steps for the post complete operation are:
			// * Add the amount to target user

			detailedData := NewRewardApplicationDetailedData{}
			err := json.Unmarshal(application.DetailedData, &detailedData)
			if err != nil {
				return err
			}

			for budgetType, detail := range detailedData {
				// Update user asset record
				if err := UserAssetRecordModel.CompleteAssetTransaction(tx, application.Applicant, budgetType, detail.Amount); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (app *Application) ListAuditLogs(db *gorm.DB) ([]*ApplicationAuditLog, error) {
	querySeg := db.Model(&ApplicationAuditLog{}).Where("application_id = ?", app.ID)
	return gormfind.Rows[ApplicationAuditLog](querySeg, nil)
}

func (app *Application) GetLatestAuditLog(db *gorm.DB) (*ApplicationAuditLog, error) {
	querySeg := db.Model(&ApplicationAuditLog{}).Where("application_id = ?", app.ID).Order("log_ts desc")
	return gormfind.Row[ApplicationAuditLog](querySeg)
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
