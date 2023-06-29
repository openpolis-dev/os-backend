package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
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

type NewRewardAssetRecord struct {
	// AssetName and Amount saves the token related info about this reward application.
	// The asset type is same with project budget type, which is used to match budget record in project / guild
	AssetType BudgetType      `json:"asset_type"`
	AssetName string          `json:"asset_name"`
	Amount    decimal.Decimal `json:"amount" sql:"type:decimal(20,8);"`
}

// NewRewardApplicationDetailedData saves detailed data for new reward applications
// Note: this struct has no DB table associated
type NewRewardApplicationDetailedData struct {
	// TargetUserWallet saves user wallet address that the reward will be sent to
	TargetUserWallet string `json:"user_wallet"`

	// Assets saves all application assets, the key for this map is asset name
	Assets map[string]NewRewardAssetRecord `json:"assets"`
}

func (detailedData *NewRewardApplicationDetailedData) AmountOfAssetType(assetType BudgetType) (decimal.Decimal, bool) {
	total := decimal.NewFromInt(0)
	found := false
	for _, record := range (*detailedData).Assets {
		if record.AssetType == assetType {
			total = total.Add(record.Amount)
			found = true
		}
	}
	return total, found
}

// FrontendApplicationRecord defines struct for application record that returns to frontend invoker
type FrontendApplicationRecord struct {
	ApplicationID    uint      `json:"application_id"`
	EntityName       string    `json:"entity_name"` // name field value from specified entity table
	CreatedAt        time.Time `json:"created_at"`
	TargetUserWallet string    `json:"target_user_wallet"`
	TokenAmount      string    `json:"token_amount"`
	CreditAmount     string    `json:"credit_amount"`
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

func (r *FrontendApplicationRecord) ToCSV() []string {
	return []string{
		r.CreatedAt.Format(time.RFC3339),
		r.TargetUserWallet,
		r.CreditAmount,
		r.TokenAmount,
		r.EntityName,
		r.BudgetSource,
		r.Comment,
		r.Status,
		r.SubmitterName,
		r.SubmitterWallet,
		r.ReviewerName,
		r.ReviewerWallet,
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

// jointAppEntityRslt saves results returned by application and project join query
type jointAppEntityRslt struct {
	Application *Application `gorm:"embedded"`
	EntityName  string       `json:"entity_name"`
}

func (r *jointAppEntityRslt) ToFrontedApplicationRecord(db *gorm.DB) *FrontendApplicationRecord {
	var tokenAmount decimal.Decimal
	var creditAmount decimal.Decimal
	var targetUserWallet string
	if r.Application.Type == ApplicationNewReward {
		detailedData := NewRewardApplicationDetailedData{}
		err := json.Unmarshal(r.Application.DetailedData, &detailedData)
		if err != nil {
			return nil
		}

		tokenAmount, _ = detailedData.AmountOfAssetType(BudgetTypeToken)
		creditAmount, _ = detailedData.AmountOfAssetType(BudgetTypeCredit)

		targetUserWallet = detailedData.TargetUserWallet
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
		TokenAmount:      tokenAmount.String(),
		CreditAmount:     creditAmount.String(),
		BudgetSource:     r.EntityName,
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

type UpdateAssetRequestParams struct {
	AssetName   string          `json:"asset_name"`
	BudgetType  BudgetType      `json:"budget_type"`
	TotalAmount decimal.Decimal `json:"total_amount" sql:"type:decimal(20,8);"`
}

type TreasuryAssetsResponse struct {
	ID                 uint            `json:"id"`
	QuarterNum         string          `json:"quarter_num"`
	CreditTotalAmount  decimal.Decimal `json:"credit_total_amount"`
	CreditRemainAmount decimal.Decimal `json:"credit_remain_amount"`
	TokenTotalAmount   decimal.Decimal `json:"token_total_amount"`
	TokenRemainAmount  decimal.Decimal `json:"token_remain_amount"`
}

// NewApplicationRequest is used to save new application request data passed from frontend
type NewApplicationRequest struct {
	Type             string          `json:"type"`
	Entity           string          `json:"entity"`
	EntityId         uint            `json:"entity_id"`
	TargetUserWallet string          `json:"target_user_wallet"`
	CreditAssetName  string          `json:"credit_asset_name"`
	CreditAmount     decimal.Decimal `json:"credit_amount"`
	TokenAssetName   string          `json:"token_asset_name"`
	TokenAmount      decimal.Decimal `json:"token_amount"`
	DetailedType     string          `json:"detailed_type"`
	Comment          string          `json:"comment"`
}
