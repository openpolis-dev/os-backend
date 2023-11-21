package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type ApplicationType string
type AuditActionType string
type ApplicationState string

const ExportApplicationTimeZone = "Asia/Shanghai"
const ExportApplicationTimeFormat = "2006-01-02 15:04:05"

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
	ApplicationStateRejected:   {AuditActionApprove: ApplicationStateApproved},
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
	AssetName  string `form:"asset_name"`
	StartDate  string `form:"start_date"`
	EndDate    string `form:"end_date"`
	Applicant  string `form:"applicant"`
	UserWallet string `form:"user_wallet"`
	SeasonId   int    `form:"season_id"`
}

type NewRewardAssetRecord struct {
	// AssetName and Amount saves the token related info about this reward application.
	// The asset type is same with project budget type, which is used to match budget record in project / guild
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

// FrontendApplicationRecord defines struct for application record that returns to frontend invoker
type FrontendApplicationRecord struct {
	ApplicationID    uint      `json:"application_id"`
	SeasonName       string    `json:"season_name"`
	EntityName       string    `json:"entity_name"` // name field value from specified entity table
	CreatedAt        time.Time `json:"-"`
	AssetName        string    `json:"asset_name"`
	Amount           string    `json:"amount"`
	BudgetSource     string    `json:"budget_source"` // the data is from name field of project or guild
	Status           string    `json:"status"`        // application status
	DetailedType     string    `json:"detailed_type"`
	Comment          string    `json:"comment"`
	AppBundleComment string    `json:"app_bundle_comment"`

	// target user data
	TargetUserWallet string `json:"target_user_wallet"`
	TargetUserAvatar string `json:"target_user_avatar"`

	// related users in the application process
	ApplicantWallet string `json:"applicant_wallet"`
	ApplicantAvatar string `json:"applicant_avatar"`
	ApplyTs         int64  `json:"apply_ts"` // The timestamp this application been created

	ReviewerWallet string `json:"reviewer_wallet"`
	ReviewerAvatar string `json:"reviewer_avatar"`
	ReviewTs       int64  `json:"review_ts"` // The timestamp this application been reviewed

	ProcessorWallet string `json:"processor_wallet"`
	ProcessorAvatar string `json:"processor_avatar"`
	ProcessTs       int64  `json:"process_ts"` // The timestamp this application been processed

	CompleterWallet string `json:"completer_wallet"`
	CompleterAvatar string `json:"completer_avatar"`
	CompleteTs      int64  `json:"complete_ts"` // The timestamp this application been marked as completed

	TransactionIds string `json:"transaction_ids"`
	CreateTs       int64  `json:"create_ts"`
	UpdateTs       int64  `json:"update_ts"`
}

func (r *FrontendApplicationRecord) ToCSV() []string {
	var createdAtStr string
	var err error
	createdAtStr, err = ConvertTimeToTzString(r.CreatedAt, ExportApplicationTimeZone, ExportApplicationTimeFormat)
	if err != nil {
		createdAtStr = r.CreatedAt.Format(time.RFC3339)
	}

	return []string{
		createdAtStr,
		r.TargetUserWallet,
		r.AssetName,
		r.Amount,
		r.DetailedType,
		r.BudgetSource,
		r.Comment,
		r.Status,
		r.ApplicantWallet,
		r.ReviewerWallet,
	}
}

func (r *FrontendApplicationRecord) ToXlsx() []any {
	var createdAtStr string
	var err error
	createdAtStr, err = ConvertTimeToTzString(r.CreatedAt, ExportApplicationTimeZone, ExportApplicationTimeFormat)
	if err != nil {
		createdAtStr = r.CreatedAt.Format(time.RFC3339)
	}

	return []any{
		createdAtStr,
		r.TargetUserWallet,
		r.AssetName,
		r.Amount,
		r.DetailedType,
		r.BudgetSource,
		r.Comment,
		r.Status,
		r.ApplicantWallet,
		r.ReviewerWallet,
	}
}

// jointAppEntityRslt saves applications records by guild and project join query
type jointAppEntityRslt struct {
	Application *Application `gorm:"embedded"`
	EntityName  string       `json:"entity_name"`
}

func (r *jointAppEntityRslt) ToFrontedApplicationRecord(db *gorm.DB) *FrontendApplicationRecord {
	var err error
	var submitterWallet string
	var reviewerWallet string

	submitterWallet = r.Application.Applicant

	auditlog := ApplicationAuditLog{}
	err = db.Model(&ApplicationAuditLog{}).
		Where(&ApplicationAuditLog{ApplicationID: r.Application.ID}).
		Where("operation IN ?", []string{AuditActionApprove, AuditActionReject}).First(&auditlog).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// No record found, skip
			log.Warn().Msgf("Application %d has no audit log found", r.Application.ID)
		} else {
			return nil
		}
	} else {
		reviewerWallet = auditlog.Operator
	}

	var appSeasonRcd Season
	err = db.Model(&Season{}).First(&appSeasonRcd, r.Application.SeasonId).Error
	if err != nil {
		log.Error().Msgf("Query season error: %+v", err)
		return nil
	}

	return &FrontendApplicationRecord{
		ApplicationID:    r.Application.ID,
		SeasonName:       appSeasonRcd.Name,
		EntityName:       r.Application.EntityType,
		CreatedAt:        r.Application.CreatedAt,
		CreateTs:         r.Application.CreateTs,
		TargetUserWallet: r.Application.TargetUserWallet,
		AssetName:        r.Application.AssetName,
		Amount:           r.Application.AssetAmount.String(),
		BudgetSource:     r.EntityName,
		Status:           string(r.Application.State),
		DetailedType:     r.Application.DetailedType,
		Comment:          r.Application.Comment,
		ApplicantWallet:  submitterWallet,
		ReviewerWallet:   reviewerWallet,
		TransactionIds:   r.Application.CompleteMessage,
	}
}

type UpdateAssetRequestParams struct {
	AssetName   string          `json:"asset_name"`
	TotalAmount decimal.Decimal `json:"total_amount" sql:"type:decimal(20,8);"`
}

type TreasuryAssetsResponse struct {
	CreditTotalAmount decimal.Decimal `json:"credit_total_amount"`
	CreditUsedAmount  decimal.Decimal `json:"credit_used_amount"`
	TokenTotalAmount  decimal.Decimal `json:"token_total_amount"`
	TokenUsedAmount   decimal.Decimal `json:"token_used_amount"`
}

// NewApplicationRequest is used to save new application request data passed from frontend
type NewApplicationRequest struct {
	Type             string          `json:"type"`
	Entity           string          `json:"entity"`
	EntityId         uint            `json:"entity_id"`
	TargetUserWallet string          `json:"target_user_wallet"`
	AssetName        string          `json:"asset_name"`
	Amount           decimal.Decimal `json:"amount"`
	DetailedType     string          `json:"detailed_type"`
	Comment          string          `json:"comment"`
}

// App Bundle related types and logic

type ListAppBundleQueryParams struct {
	Page      int    `form:"page"`
	Size      int    `form:"size"`
	SortField string `form:"sort_field"`
	SortOrder string `form:"sort_order"`
	State     string `form:"state"`
	Entity    string `form:"entity"`
	EntityId  string `form:"entity_id"`
	Applicant string `form:"applicant"`
	SeasonId  int    `form:"season_id"`
}

// JointAppBundleEntityRslt saves app bundles records by guild and project join query
type JointAppBundleEntityRslt struct {
	AppBundle  *AppBundle `gorm:"embedded"`
	EntityName string     `json:"entity_name"`
	SeasonName string     `json:"season_name"`
}

func ToFrontendApplicationRecordList(db *gorm.DB, appRcds []*Application, entityName string) []*FrontendApplicationRecord {
	return lo.Map(appRcds, func(appRcd *Application, _ int) *FrontendApplicationRecord {
		appEntityRcd := jointAppEntityRslt{
			Application: appRcd,
			EntityName:  entityName,
		}
		return appEntityRcd.ToFrontedApplicationRecord(db)
	})
}

// NewAppBundleRequest saves new application bundle request data, the records inside uses NewApplicationRequest directly
// The only type in newAppBundle is newApplication is newReward
type NewAppBundleRequest struct {
	Entity   string                   `json:"entity"`
	EntityId uint                     `json:"entity_id"`
	Comment  string                   `json:"comment"`
	Records  []*NewApplicationRequest `json:"records"`
}
