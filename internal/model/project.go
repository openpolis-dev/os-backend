package model

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type ProjectStatus string

const (
	ProjectStatusOpen         ProjectStatus = "open"
	ProjectStatusPendingClose               = "pending_close"
	ProjectStatusClosed                     = "closed"
)

type Project struct {
	ID        uint          `json:"id" gorm:"primaryKey"`
	Logo      string        `json:"logo"`
	Name      string        `json:"name"`
	Status    ProjectStatus `json:"status"` // Status may have those values: open/pending_close/closed
	Sponsors  []string      `json:"sponsors" gorm:"serializer:json"`
	Members   []string      `json:"members" gorm:"serializer:json"`
	Proposals []string      `json:"proposals" gorm:"serializer:json"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type projectModel struct{}

var ProjectModel projectModel

func (*projectModel) CreateOrUpdate(db *gorm.DB, proj *Project) error {
	return db.Save(proj).Error
}

func (*projectModel) Detail(db *gorm.DB, id uint) (*Project, error) {
	querySeg := db.Where("id = ?", id)
	return gormfind.Row[Project](querySeg)
}

func (*projectModel) List(db *gorm.DB, status string, page *gormfind.Page) (data []*Project, total int64, err error) {
	querySeg := db.Table("projects")
	if status != "" {
		querySeg.Where("status = ?", status)
	}

	total, err = gormfind.Count(querySeg)
	if err != nil {
		return
	}

	data, err = gormfind.Rows[Project](querySeg, page)
	if err != nil {
		return
	}

	return data, total, nil
}

func (*projectModel) ListBySponsorOrMember(db *gorm.DB, wallet string, page *gormfind.Page) (data []*Project, total int64, err error) {
	w := fmt.Sprintf("%%\"%s\"%%", wallet) // value is: `%"0x123"%`
	querySeg := db.Table("projects").Where("sponsors LIKE ?", w).Or("members LIKE ?", w)

	total, err = gormfind.Count(querySeg)
	if err != nil {
		return
	}
	data, err = gormfind.Rows[Project](querySeg, page)
	if err != nil {
		return
	}
	return data, total, nil
}

// SetBudget set budget record directly. Only totalAmount will be passed in.
// If the budget is not existing, a new record will be created with total and remain amount all set to passed in value
// If the budget is already existing, the total will be updated to passed in value, and the remain will also be updated by the delta
func (*projectModel) SetBudget(db *gorm.DB, projectId uint, budgetType BudgetType, assertName string, totalAmount decimal.Decimal) error {
	return db.Transaction(func(tx *gorm.DB) error {
		budgetRecord, err := ProjectBudgetModel.QueryByProjectIdAndBudgetProps(tx, projectId, budgetType, assertName)
		if err != nil {
			return err
		}

		if budgetRecord == nil {
			budgetRecord = &ProjectBudget{
				ProjectID:    projectId,
				AssetName:    assertName,
				Type:         budgetType,
				TotalAmount:  totalAmount,
				RemainAmount: totalAmount,
			}
		} else {
			usedAmount := budgetRecord.TotalAmount.Sub(budgetRecord.RemainAmount)
			budgetRecord.TotalAmount = totalAmount
			budgetRecord.RemainAmount = totalAmount.Sub(usedAmount)
		}

		return ProjectBudgetModel.Update(tx, budgetRecord)
	})
}

func (*projectModel) WithdrawBudget(db *gorm.DB, projectId uint, budgetType BudgetType, assetName string, tokenAmount decimal.Decimal) error {
	return db.Transaction(func(tx *gorm.DB) error {
		budgetRcd, err := ProjectBudgetModel.QueryByProjectIdAndBudgetProps(tx, projectId, budgetType, assetName)
		if err != nil {
			return err
		}

		if budgetRcd == nil {
			return fmt.Errorf("project %d has no budget record with asset %s", projectId, assetName)
		}

		budgetRcd.RemainAmount = budgetRcd.RemainAmount.Sub(tokenAmount)
		return tx.Save(budgetRcd).Error
	})
}

// DepositBudget deposits budget back to project, e.g. application for reward has been rejected
func (*projectModel) DepositBudget(db *gorm.DB, projectId uint, budgetType BudgetType, assetName string, tokenAmount decimal.Decimal) error {
	return db.Transaction(func(tx *gorm.DB) error {
		budgetRcd, err := ProjectBudgetModel.QueryByProjectIdAndBudgetProps(tx, projectId, budgetType, assetName)
		if err != nil {
			return err
		}

		if budgetRcd == nil {
			return tx.Save(&ProjectBudget{
				ProjectID:    projectId,
				AssetName:    assetName,
				Type:         budgetType,
				TotalAmount:  tokenAmount,
				RemainAmount: tokenAmount,
			}).Error
		} else {
			budgetRcd.RemainAmount = budgetRcd.RemainAmount.Add(tokenAmount)
			return tx.Save(budgetRcd).Error
		}
	})
}
