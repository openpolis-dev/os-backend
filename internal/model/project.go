package model

import (
	"fmt"
	"time"

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
	tx := db.Save(proj)
	return tx.Error
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

func (*projectModel) ListBySponsorOrMember(db *gorm.DB, wallet string, page *gormfind.Page) ([]*Project, error) {
	w := fmt.Sprintf("%%\"%s\"%%", wallet) // value is: `%"0x123"%`
	querySeg := db.Table("projects").Where("sponsors LIKE ?", w).Or("members LIKE ?", w)
	return gormfind.Rows[Project](querySeg, page)
}

// SetBudget set budget record directly, but only total amount is allowed to set directly
func (*projectModel) SetBudget(db *gorm.DB, projectId uint, assertName string, totalAmount uint64) error {
	budgetRecord, err := ProjectBudgetModel.QueryByProjectIdAndAssetName(db, projectId, assertName)
	if err != nil {
		return err
	}

	if budgetRecord == nil {
		budgetRecord = &ProjectBudget{
			ProjectID:    projectId,
			Name:         assertName,
			TotalAmount:  totalAmount,
			RemainAmount: totalAmount,
		}
	} else {
		budgetRecord.TotalAmount = totalAmount
	}

	return ProjectBudgetModel.Update(db, budgetRecord)
}

func (*projectModel) WithdrawBudget(db *gorm.DB, projectId uint, tokenName string, tokenAmount uint64) error {
	budgetRcd, err := ProjectBudgetModel.QueryByProjectIdAndAssetName(db, projectId, tokenName)
	if err != nil {
		return err
	}

	if budgetRcd == nil {
		return fmt.Errorf("project %d has no budget record with asset %s", projectId, tokenName)
	}

	if budgetRcd.RemainAmount < tokenAmount {
		return fmt.Errorf("project %d has insufficient budget record with asset %s", projectId, tokenName)
	}

	budgetRcd.RemainAmount -= tokenAmount
	return db.Save(budgetRcd).Error
}

// DepositBudget deposits budget back to project, e.g. application for reward has been rejected
func (*projectModel) DepositBudget(db *gorm.DB, projectId uint, tokenName string, tokenAmount uint64) error {
	budgetRcd, err := ProjectBudgetModel.QueryByProjectIdAndAssetName(db, projectId, tokenName)
	if err != nil {
		return err
	}

	if budgetRcd == nil {
		return db.Save(&ProjectBudget{
			ProjectID:    projectId,
			Name:         tokenName,
			TotalAmount:  tokenAmount,
			RemainAmount: tokenAmount,
		}).Error
	} else {
		budgetRcd.RemainAmount += tokenAmount
		return db.Save(budgetRcd).Error
	}
}
