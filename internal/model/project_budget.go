package model

import (
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type ProjectBudget struct {
	gorm.Model

	ProjectID    uint   `json:"projectID"` // project_id
	Name         string `json:"name"`
	TotalAmount  uint64 `json:"totalAmount"`  // total_amount
	RemainAmount uint64 `json:"remainAmount"` // remain_amount
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
