package service

import (
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type projectBudgetModel struct{}

var ProjectBudgetModel projectBudgetModel

func (*projectBudgetModel) Create(db *gorm.DB, budgets []*model.ProjectBudget) error {
	tx := db.Create(budgets)
	return tx.Error
}

func (*projectBudgetModel) Update(db *gorm.DB, budget *model.ProjectBudget) error {
	return db.Save(budget).Error
}

func (*projectBudgetModel) Detail(db *gorm.DB, id uint) (*model.ProjectBudget, error) {
	querySeg := db.Where("id = ?", id)
	return gormfind.Row[model.ProjectBudget](querySeg)
}

func (*projectBudgetModel) ListByProjectId(db *gorm.DB, projID uint) ([]*model.ProjectBudget, error) {
	querySeg := db.Where("project_id = ?", projID)
	return model.QueryRows[model.ProjectBudget](querySeg, nil)
}

func (*projectBudgetModel) QueryByProjectIdAndBudgetProps(db *gorm.DB, projID uint, assetName string) (*model.ProjectBudget, error) {
	querySeg := db.Where(&model.ProjectBudget{ProjectID: projID, AssetName: assetName})
	return gormfind.Row[model.ProjectBudget](querySeg)
}
