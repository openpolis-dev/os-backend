package model

import (
	"time"

	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type ProjectBudget struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	ProjectID    uint      `json:"project_id"` // project_id
	Name         string    `json:"name"`
	TotalAmount  uint64    `json:"total_amount"`  // total_amount
	RemainAmount uint64    `json:"remain_amount"` // remain_amount
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type projectBudgetModel struct{}

var ProjectBudgetModel projectBudgetModel

func (*projectBudgetModel) Create(db *gorm.DB, budgets []*ProjectBudget) error {
	tx := db.Create(budgets)
	return tx.Error
}

func (*projectBudgetModel) Update(db *gorm.DB, budget *ProjectBudget) error {
	tx := db.Save(budget)
	return tx.Error
}

func (*projectBudgetModel) Detail(db *gorm.DB, id uint) (*ProjectBudget, error) {
	querySeg := db.Where("id = ?", id)
	return gormfind.Row[ProjectBudget](querySeg)
}

func (*projectBudgetModel) ListByProjectId(db *gorm.DB, projID uint) ([]*ProjectBudget, error) {
	querySeg := db.Where("project_id = ?", projID)
	return gormfind.Rows[ProjectBudget](querySeg, nil)
}

func (*projectBudgetModel) QueryByProjectIdAndAssetName(db *gorm.DB, projID uint, assetName string) (*ProjectBudget, error) {
	querySeg := db.Where("project_id = ?", projID).Where("name = ", assetName)
	return gormfind.Row[ProjectBudget](querySeg)
}
