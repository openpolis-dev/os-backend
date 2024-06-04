package model

import (
	"time"

	"github.com/shopspring/decimal"
)

type ProjectBudget struct {
	ID         uint `json:"id" gorm:"primaryKey"`
	ProposalID uint `json:"proposal_id"` // Proposal ID that creating this project
	ProjectID  uint `json:"project_id"`  // project_id

	AssetName    string          `json:"asset_name"`
	TotalAmount  decimal.Decimal `json:"total_amount" sql:"type:decimal(20,8);"`  // total_amount = used_amount + remain_amount
	UsedAmount   decimal.Decimal `json:"used_amount" sql:"type:decimal(20,8);"`   // used_amount
	RemainAmount decimal.Decimal `json:"remain_amount" sql:"type:decimal(20,8);"` // remain_amount

	AdvanceRatio        decimal.Decimal `json:"advance_ratio" sql:"type:decimal(7,3);"` // How many assets can be paid in advanced, value range 0.0-1.0
	TotalAdvanceAmount  decimal.Decimal `json:"total_advance_amount" sql:"type:decimal(20,8);"`
	UsedAdvanceAmount   decimal.Decimal `json:"used_advance_amount" sql:"type:decimal(20,8);"`
	RemainAdvanceAmount decimal.Decimal `json:"remain_advance_amount" sql:"type:decimal(20,8);"`

	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
	CreateTs  int64     `json:"create_ts" gorm:"index"`
	UpdateTs  int64     `json:"update_ts" gorm:"index"`
}
