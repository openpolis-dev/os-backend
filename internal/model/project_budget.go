package model

import (
	"time"

	"github.com/shopspring/decimal"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type BudgetType string

const (
	BudgetTypeCredit BudgetType = "credit"
	BudgetTypeToken             = "token"
)

type ProjectBudget struct {
	ID           uint            `json:"id" gorm:"primaryKey"`
	ProjectID    uint            `json:"project_id"` // project_id
	AssetName    string          `json:"name"`
	Type         BudgetType      `json:"type"`                                    // budget type, credit or token
	TotalAmount  decimal.Decimal `json:"total_amount" sql:"type:decimal(20,8);"`  // total_amount
	RemainAmount decimal.Decimal `json:"remain_amount" sql:"type:decimal(20,8);"` // remain_amount
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
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
	return gormfind.Rows[ProjectBudget](querySeg, nil)
}

func (*projectBudgetModel) QueryByProjectIdAndBudgetProps(db *gorm.DB, projID uint, budgetType BudgetType, assetName string) (*ProjectBudget, error) {
	querySeg := db.Where(&ProjectBudget{ProjectID: projID, AssetName: assetName, Type: budgetType})
	return gormfind.Row[ProjectBudget](querySeg)
}
