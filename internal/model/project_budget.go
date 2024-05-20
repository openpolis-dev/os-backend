package model

import (
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal/api/project"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type ProjectBudget struct {
	ID         uint `json:"id" gorm:"primaryKey"`
	ProposalID uint `json:"proposal_id"` // Proposal ID that creating this project
	ProjectID  uint `json:"project_id"`  // project_id

	AssetName    string          `json:"name"`
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

type projectBudgetModel struct{}

var ProjectBudgetModel projectBudgetModel

func (*projectBudgetModel) Create(db *gorm.DB, budgets []*ProjectBudget) error {
	tx := db.Create(budgets)
	return tx.Error
}

func (*projectBudgetModel) Update(db *gorm.DB, budget *ProjectBudget) error {
	return db.Save(budget).Error
}

func (*projectBudgetModel) Detail(db *gorm.DB, id uint) (*ProjectBudget, error) {
	querySeg := db.Where("id = ?", id)
	return gormfind.Row[ProjectBudget](querySeg)
}

func (*projectBudgetModel) ListByProjectId(db *gorm.DB, projID uint) ([]*ProjectBudget, error) {
	querySeg := db.Where("project_id = ?", projID)
	return QueryRows[ProjectBudget](querySeg, nil)
}

func (*projectBudgetModel) QueryByProjectIdAndBudgetProps(db *gorm.DB, projID uint, assetName string) (*ProjectBudget, error) {
	querySeg := db.Where(&ProjectBudget{ProjectID: projID, AssetName: assetName})
	return gormfind.Row[ProjectBudget](querySeg)
}

func (*projectBudgetModel) BuildBudgetResponse(budgetRcds []*ProjectBudget) []*project.BudgetResp {
	return lo.Map(budgetRcds, func(r *ProjectBudget, _ int) *project.BudgetResp {
		return &project.BudgetResp{
			AssetName:           r.AssetName,
			TotalAmount:         r.TotalAmount.String(),
			UsedAmount:          r.UsedAmount.String(),
			RemainAmount:        r.RemainAmount.String(),
			AdvanceRatio:        r.AdvanceRatio.String(),
			TotalAdvanceAmount:  r.TotalAdvanceAmount.String(),
			UsedAdvanceAmount:   r.UsedAdvanceAmount.String(),
			RemainAdvanceAmount: r.RemainAdvanceAmount.String(),
		}
	})
}
