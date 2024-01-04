package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type ProjectStatus string

type SpecialProjectType string

const (
	SpecialProjectCityHall SpecialProjectType = "city_hall"
)

const (
	ProjectStatusOpen         ProjectStatus = "open"
	ProjectStatusPendingClose               = "pending_close"
	ProjectStatusClosed                     = "closed"
)

type Project struct {
	ID              uint                `json:"id" gorm:"primaryKey"`
	Logo            string              `json:"logo"`
	Name            string              `json:"name"`
	Intro           string              `json:"intro"`
	Desc            string              `json:"desc"`
	Status          ProjectStatus       `json:"status" gorm:"index"` // Status may have those values: open/pending_close/closed
	GroupedSponsors map[string][]string `json:"grouped_sponsors" gorm:"serializer:json"`
	Sponsors        []string            `json:"sponsors" gorm:"serializer:json"`
	Members         []string            `json:"members" gorm:"serializer:json"`
	Proposals       []string            `json:"proposals" gorm:"serializer:json"`

	Creator string `json:"creator"`

	IsSpecial   bool               `json:"is_special" gorm:"index"`
	SpecialType SpecialProjectType `json:"special_type" gorm:"index"`

	CreatedAt time.Time `json:"-" gorm:"index"`
	UpdatedAt time.Time `json:"-"`
	CreateTs  int64     `json:"create_ts" gorm:"index"`
	UpdateTs  int64     `json:"update_ts" gorm:"index"`
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

func (*projectModel) List(db *gorm.DB, status string, page *gormfind.Page, showSpecialProjectFlag bool) (data []*Project, total int64, err error) {
	querySeg := db.Table("projects")
	if !showSpecialProjectFlag {
		querySeg = querySeg.Where("is_special = false")
	}
	if status != "" {
		if strings.Contains(status, ",") {
			querySeg = querySeg.Where("status IN ?", strings.Split(status, ","))
		} else {
			querySeg = querySeg.Where("status = ?", status)
		}
	}

	total, err = gormfind.Count(querySeg)
	if err != nil {
		return
	}

	data, err = QueryRows[Project](querySeg, page)
	if err != nil {
		return
	}

	return data, total, nil
}

func (*projectModel) ListBySponsorOrMember(db *gorm.DB, wallet string, page *gormfind.Page) (data []*Project, total int64, err error) {
	w := fmt.Sprintf("%%\"%s\"%%", wallet) // value is: `%"0x123"%`
	// MySQL version
	//querySeg := db.Table("projects").Where("sponsors LIKE ?", w).Or("members LIKE ?", w)

	// PgVersion
	querySeg := db.Table("projects").Where(fmt.Sprintf("sponsors::text ILIKE '%%%s%%'", w)).Or(fmt.Sprintf("members::text ILIKE '%%%s%%'", w))

	total, err = gormfind.Count(querySeg)
	if err != nil {
		return
	}
	data, err = QueryRows[Project](querySeg, page)
	if err != nil {
		return
	}
	return data, total, nil
}

func (*projectModel) ListBySponsor(db *gorm.DB, sponsor string, status string, page *gormfind.Page, showSpecialProjectFlag bool) (data []*Project, total int64, err error) {
	querySeg := db.Table("projects").Where(fmt.Sprintf("sponsors ILIKE '%%%s%%'", sponsor)) // value is: `%"0x123"%`
	if !showSpecialProjectFlag {
		querySeg = querySeg.Where("is_special = false")
	}
	if status != "" {
		if strings.Contains(status, ",") {
			querySeg = querySeg.Where("status IN ?", strings.Split(status, ","))
		} else {
			querySeg = querySeg.Where("status = ?", status)
		}
	}

	total, err = gormfind.Count(querySeg)
	if err != nil {
		return
	}
	data, err = QueryRows[Project](querySeg, page)
	if err != nil {
		return
	}
	return data, total, nil
}

// SetBudget set budget record directly. Only totalAmount will be passed in.
// If the budget is not existing, a new record will be created with total and remain amount all set to passed in value
// If the budget is already existing, the total will be updated to passed in value, and the remain will also be updated by the delta
func (*projectModel) SetBudget(db *gorm.DB, projectId uint, assertName string, totalAmount decimal.Decimal) error {
	return db.Transaction(func(tx *gorm.DB) error {
		budgetRecord, err := ProjectBudgetModel.QueryByProjectIdAndBudgetProps(tx, projectId, assertName)
		if err != nil {
			return err
		}

		if budgetRecord == nil {
			budgetRecord = &ProjectBudget{
				ProjectID:    projectId,
				AssetName:    assertName,
				TotalAmount:  totalAmount,
				UsedAmount:   decimal.Zero,
				RemainAmount: totalAmount,
			}
		} else {
			budgetRecord.TotalAmount = totalAmount
			budgetRecord.RemainAmount = totalAmount.Sub(budgetRecord.UsedAmount)
		}

		return ProjectBudgetModel.Update(tx, budgetRecord)
	})
}

func (*projectModel) WithdrawBudget(db *gorm.DB, projectId uint, assetName string, tokenAmount decimal.Decimal) error {
	return db.Transaction(func(tx *gorm.DB) error {
		budgetRcd, err := ProjectBudgetModel.QueryByProjectIdAndBudgetProps(tx, projectId, assetName)
		if err != nil {
			return err
		}

		if budgetRcd == nil {
			return fmt.Errorf("project %d has no budget record with asset %s", projectId, assetName)
		}

		budgetRcd.UsedAmount = budgetRcd.UsedAmount.Add(tokenAmount)
		budgetRcd.RemainAmount = budgetRcd.RemainAmount.Sub(tokenAmount)
		return tx.Save(budgetRcd).Error
	})
}

// DepositBudget deposits budget back to project, e.g. application for reward has been rejected
func (*projectModel) DepositBudget(db *gorm.DB, projectId uint, assetName string, tokenAmount decimal.Decimal) error {
	return db.Transaction(func(tx *gorm.DB) error {
		budgetRcd, err := ProjectBudgetModel.QueryByProjectIdAndBudgetProps(tx, projectId, assetName)
		if err != nil {
			return err
		}

		if budgetRcd == nil {
			return tx.Save(&ProjectBudget{
				ProjectID:    projectId,
				AssetName:    assetName,
				TotalAmount:  tokenAmount,
				UsedAmount:   decimal.Zero,
				RemainAmount: tokenAmount,
			}).Error
		} else {
			budgetRcd.UsedAmount = budgetRcd.UsedAmount.Sub(tokenAmount)
			budgetRcd.RemainAmount = budgetRcd.RemainAmount.Add(tokenAmount)
			return tx.Save(budgetRcd).Error
		}
	})
}

// GetCityHallProject get cityhall project in DB
func GetCityHallProject(db *gorm.DB) (*Project, error) {
	project := Project{}
	db.Where(Project{
		IsSpecial:   true,
		SpecialType: SpecialProjectCityHall,
	}).First(&project)
	return &project, nil
}

// GetOrCreateCityHallProject get or create cityhall project in DB
func GetOrCreateCityHallProject(db *gorm.DB, cityHallUsers []string) (*Project, error) {
	project := Project{}
	db.Where(Project{
		IsSpecial:   true,
		SpecialType: SpecialProjectCityHall,
	}).First(&project)

	if project.ID == 0 {
		generatedProject, err := createCityHallProject(db, cityHallUsers)

		if err != nil {
			return nil, err
		}

		project = *generatedProject
	}

	return &project, nil
}

func createCityHallProject(db *gorm.DB, cityHallUsers []string) (*Project, error) {
	project := Project{
		Name:        "CityHall",
		IsSpecial:   true,
		SpecialType: SpecialProjectCityHall,
		Sponsors:    cityHallUsers,
		CreatedAt:   time.Now().In(internal.ProjectTimezone),
		UpdatedAt:   time.Now().In(internal.ProjectTimezone),
		CreateTs:    GetCurrentUtcEpochSecond(),
		UpdateTs:    GetCurrentUtcEpochSecond(),
	}
	err := db.Create(&project).Error

	if err != nil {
		return nil, err
	}
	return &project, nil
}
