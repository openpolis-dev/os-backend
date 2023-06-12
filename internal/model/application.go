package model

// Application model saves all applications made by user, which includes close_project
// and new_reward currently.

import (
	"time"

	"gorm.io/gorm"
)

// TODO: Make sure whether enum logic can be implemented in golang side
//type applicationType string
//type applicationState string
//
//const (
//	CloseProject applicationType = "CLOSE_PROJECT"
//	NewReward    applicationType = "NEW_REWARD"
//)

type Application struct {
	gorm.Model

	// unique ID for this request
	ID uint `json:"id"`

	// application type, should be enum
	Type string `json:"type"`

	// Member send this application
	Applicant string `json:"applicant"`

	// Application state, which contains open/approved/rejected/processing/completed
	State string `json:"state"`

	// Saves the reject reason if this application state is rejected
	RejectReason string `json:"reject_reason"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Detailed data saves application specified data
	DetailedData string `json:"detailed_data"`

	AuditLogs []ApplicationAuditLog `json:"audit_logs"`
}

type ApplicationAuditLog struct {
	gorm.Model

	// unique ID of this audit log
	ID uint `json:"id"`

	// Which application this audit log belongs to
	ApplicationID uint `json:"application_id"`

	LogTs time.Time `json:"log_ts"`

	// Which operation this log record, which should be in approve/reject/process/complete
	Operation string `json:"operation"`

	// Who perform this operation
	Operator User `json:"operator"`

	// Application state before this operation
	PreState string `json:"pre_state"`

	// Application state after this operation
	PostState string `json:"post_state"`
}

// NewApplicationRecord create application and related audit log message with given params
func NewApplicationRecord() {

}
