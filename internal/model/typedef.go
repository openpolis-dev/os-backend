package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type ApplicationType string
type AuditActionType string
type ApplicationState string

func ParseApplicationType(typeStr string) (ApplicationType, error) {
	switch strings.ToUpper(typeStr) {
	case "CLOSE_PROJECT":
		return ApplicationCloseProject, nil
	case "NEW_REWARD":
		return ApplicationNewReward, nil
	default:
		return "", fmt.Errorf("unknown application type %s", typeStr)
	}
}
func MustParseApplicationType(typeStr string) ApplicationType {
	applicationType, err := ParseApplicationType(typeStr)
	if err != nil {
		panic(err)
	}
	return applicationType
}

func (t ApplicationType) ToString() string {
	return string(t)
}

const (
	ApplicationCloseProject ApplicationType = "CLOSE_PROJECT"
	ApplicationNewReward    ApplicationType = "NEW_REWARD"
)

const (
	AuditActionNew      AuditActionType = "new"
	AuditActionApprove                  = "approve"
	AuditActionReject                   = "reject"
	AuditActionProcess                  = "process"
	AuditActionComplete                 = "complete"
)

const (
	ApplicationStateOpen       ApplicationState = "open"
	ApplicationStateApproved                    = "approved"
	ApplicationStateRejected                    = "rejected"
	ApplicationStateProcessing                  = "processing"
	ApplicationStateCompleted                   = "completed"
)

// This variable saves state transit map for all application states
var applicationStateMap = map[ApplicationState]map[AuditActionType]ApplicationState{
	ApplicationStateOpen:       {AuditActionApprove: ApplicationStateApproved, AuditActionReject: ApplicationStateRejected},
	ApplicationStateApproved:   {AuditActionProcess: ApplicationStateProcessing},
	ApplicationStateRejected:   {},
	ApplicationStateProcessing: {AuditActionComplete: ApplicationStateCompleted},
	ApplicationStateCompleted:  {},
}

type ListApplicationQueryParams struct {
	Page       int    `form:"page"`
	Size       int    `form:"size"`
	SortField  string `form:"sort_field"`
	SortOrder  string `form:"sort_order"`
	State      string `form:"state"`
	Type       string `form:"type"`
	Entity     string `form:"entity"`
	EntityId   string `form:"entity_id"`
	StartDate  string `form:"start_date"`
	EndDate    string `form:"end_date"`
	Applicant  string `form:"applicant"`
	UserWallet string `form:"user_wallet"`
}

// rewardDetail saves detail of reward application for single budget type
type rewardDetail struct {
	ApplicationID uint `json:"application_id"`

	// TargetUserWallet saves user wallet address that the reward will be sent to
	TargetUserWallet string `json:"user_wallet"`

	// AssetName and Amount saves the token related info about this reward application.
	// The asset type is same with project budget type, which is used to match budget record in project / guild
	AssetType BudgetType `json:"asset_type"`
	AssetName string     `json:"asset_name"`
	Amount    uint64     `json:"amount"`
}

// NewRewardApplicationDetailedData saves detailed data for new reward applications
// Note: this struct has no DB table associated
type NewRewardApplicationDetailedData map[BudgetType]rewardDetail

func (detailedData *NewRewardApplicationDetailedData) AmountOfBudgetType(budgetType BudgetType) (uint64, bool) {
	if r, found := (*detailedData)[budgetType]; found {
		return r.Amount, true
	} else {
		return 0, false
	}
}
func (detailedData *NewRewardApplicationDetailedData) GetTargetUserWallet() string {
	for _, rcd := range *detailedData {
		return rcd.TargetUserWallet
	}
	return ""
}

// FrontendApplicationRecord defines struct for application record that returns to frontend invoker
type FrontendApplicationRecord struct {
	ApplicationID    uint      `json:"application_id"`
	EntityName       string    `json:"entity_name"` // name field value from specified entity table
	CreatedAt        time.Time `json:"created_at"`
	TargetUserWallet string    `json:"target_user_wallet"`
	TokenAmount      uint64    `json:"token_amount"`
	CreditAmount     uint64    `json:"credit_amount"`
	BudgetSource     string    `json:"budget_source"` // the data is from name field of project or guild
	Status           string    `json:"status"`        // application status
	DetailedType     string    `json:"detailed_type"`
	Comment          string    `json:"comment"`
	SubmitterWallet  string    `json:"submitter_wallet"`
	SubmitterName    string    `json:"submitter_name"`
	ReviewerWallet   string    `json:"reviewer_wallet"`
	ReviewerName     string    `json:"reviewer_name"`
	TransactionIds   string    `json:"transaction_ids"`
}

var FrontendApplicationRecordCsvHeader = []string{
	"application_id",
	"entity_name",
	"created_at",
	"target_user_wallet",
	"token_amount",
	"credit_amount",
	"budget_source",
	"status",
	"detailed_type",
	"comment",
	"submitter_wallet",
	"submitter_name",
	"reviewer_wallet",
	"reviewer_name",
	"transaction_ids",
}

func (r *FrontendApplicationRecord) ToCSV() []string {
	return []string{
		fmt.Sprintf("%d", r.ApplicationID),
		r.EntityName,
		r.CreatedAt.Format(time.RFC3339),
		r.TargetUserWallet,
		fmt.Sprintf("%d", r.TokenAmount),
		fmt.Sprintf("%d", r.CreditAmount),
		r.BudgetSource,
		r.Status,
		r.DetailedType,
		r.Comment,
		r.SubmitterWallet,
		r.SubmitterName,
		r.ReviewerWallet,
		r.ReviewerName,
		r.TransactionIds,
	}
}

// jointAppProjectFields saves query fields of join query of application and project
const jointAppProjectFields = `applications.id,
applications.type,
applications.applicant,
applications.state,
applications.reject_reason,
applications.complete_message,
applications.created_at,
applications.updated_at,
applications.entity_type,
applications.entity_id,
applications.detailed_data,
applications.detailed_type,
applications.comment,
projects.name as prj_name,
projects.id as prj_id`

// jointAppProjectRslt saves results returned by application and project join query
type jointAppProjectRslt struct {
	Project     *Project     `gorm:"embedded;embeddedPrefix:prj_"`
	Application *Application `gorm:"embedded"`
}

func (r *jointAppProjectRslt) ToFrontedApplicationRecord(db *gorm.DB) *FrontendApplicationRecord {
	var tokenAmount uint64
	var creditAmount uint64
	var targetUserWallet string
	if r.Application.Type == ApplicationNewReward {
		detailedData := NewRewardApplicationDetailedData{}
		err := json.Unmarshal(r.Application.DetailedData, &detailedData)
		if err != nil {
			return nil
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
	submitterUsername, err := UserModel.TryGetUsername(db, submitterWallet)
	if err != nil {
		return nil
	}

	auditlog := ApplicationAuditLog{}
	err = db.Model(&ApplicationAuditLog{}).
		Where(&ApplicationAuditLog{ApplicationID: r.Application.ID}).
		Where("operation IN ?", []string{AuditActionApprove, AuditActionReject}).First(&auditlog).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// No record found, skip
		} else {
			return nil
		}
	} else {
		reviewerWallet = auditlog.Operator
		reviewerUsername, err = UserModel.TryGetUsername(db, reviewerWallet)
		if err != nil {
			return nil
		}
	}

	return &FrontendApplicationRecord{
		ApplicationID:    r.Application.ID,
		EntityName:       r.Application.EntityType,
		CreatedAt:        r.Application.CreatedAt,
		TargetUserWallet: targetUserWallet,
		TokenAmount:      tokenAmount,
		CreditAmount:     creditAmount,
		BudgetSource:     r.Project.Name,
		Status:           string(r.Application.State),
		DetailedType:     r.Application.DetailedType,
		Comment:          r.Application.Comment,
		SubmitterWallet:  submitterWallet,
		SubmitterName:    submitterUsername,
		ReviewerWallet:   reviewerWallet,
		ReviewerName:     reviewerUsername,
		TransactionIds:   r.Application.CompleteMessage,
	}
}

// Guild related code are placeholder currently
const jointAppGuildFields = `applications.id,
applications.type,
applications.applicant,
applications.state,
applications.reject_reason,
applications.complete_message,
applications.created_at,
applications.updated_at,
applications.entity_type,
applications.entity_id,
applications.detailed_data,
applications.detailed_type,
applications.comment,
guilds.name as guild_name,
guilds.id as guild_id`

type jointAppGuildRslt struct {
	//Guild     *Guild     `gorm:"embedded;embeddedPrefix:guild_"`
	Application *Application `gorm:"embedded"`
}

func (r *jointAppGuildRslt) ToFrontedApplicationRecord(db *gorm.DB) *FrontendApplicationRecord {
	return nil
}

type UpdateAssetRequestParams struct {
	AssetName   string     `json:"asset_name"`
	BudgetType  BudgetType `json:"budget_type"`
	TotalAmount uint64     `json:"total_amount"`
}

type TreasuryAssetsResponse struct {
	ID                 uint   `json:"id"`
	QuarterNum         string `json:"quarter_num"`
	CreditTotalAmount  uint64 `json:"credit_total_amount"`
	CreditRemainAmount int64  `json:"credit_remain_amount"`
	TokenTotalAmount   uint64 `json:"token_total_amount"`
	TokenRemainAmount  int64  `json:"token_remain_amount"`
}
