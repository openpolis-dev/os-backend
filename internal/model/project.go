package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
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
	ProjectStatusClosing                    = "closing"
	ProjectStatusCloseFailed                = "close_failed"
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

	Label string `json:"label"`

	SIP          string `json:"SIP" gorm:"index"`
	Category     string `json:"Category"`
	ApprovalLink string `json:"ApprovalLink"`
	OverLink     string `json:"OverLink"`
	Budgets      string `json:"Budgets"`
	Deliverable  string `json:"Deliverable"`
	PlanTime     string `json:"PlanTime"`
	ContantWay   string `json:"ContantWay"`
	OfficialLink string `json:"OfficialLink"`
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

func (*projectModel) ListWithSearch(db *gorm.DB, status string, keywords *string, wallet *string, page *gormfind.Page, showSpecialProjectFlag bool) (data []*Project, total int64, err error) {
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
	// <--- search conditions --->
	if keywords != nil {
		querySeg.Where(fmt.Sprintf("name ILIKE '%%%s%%'", *keywords))
	}
	if wallet != nil {
		w := fmt.Sprintf("%%\"%s\"%%", *wallet) // value is: `%0x123%`
		querySeg.Where(fmt.Sprintf("sponsors::text ILIKE '%%%s%%'", w))
	}
	// <--- search conditions --->

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
	querySeg := db.Table("projects").Where("is_special = false").Where(
		db.Table("projects").Where(fmt.Sprintf("sponsors::text ILIKE '%%%s%%'", w)).Or(fmt.Sprintf("members::text ILIKE '%%%s%%'", w)),
	)

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

// GetClosableProject get projects which can be closed, which means the project status should be open or close failed,
// If passing sponsor wallet, only the projects which first sponsor is given wallet will be returned
// If passing categories, only the projects with given categories will be returned
func (*projectModel) GetClosableProject(db *gorm.DB, sponsorWallet string, categories []string) (data []*Project, err error) {
	querySeg := db.Table("projects").Where("status IN ?", []string{string(ProjectStatusOpen), ProjectStatusCloseFailed})
	if sponsorWallet != "" {
		// Query projects which first sponsor is given wallet
		querySeg = querySeg.Where(fmt.Sprintf("sponsors ILIKE '[\"%s\"%%'", sponsorWallet))
	}
	if len(categories) > 0 {
		querySeg = querySeg.Where("category IN ?", categories)
	}

	data, err = QueryRows[Project](querySeg, nil)
	if err != nil {
		return
	}
	return data, nil
}

func (*projectModel) GetCommonProjects(db *gorm.DB) (data []*Project, err error) {
	querySeg := db.Table("projects").Where("category IN ?", []string{internal.ManuallyCreatedCommonProjectCategory, internal.AutomationCreatedCommonProjectCategory})
	data, err = QueryRows[Project](querySeg, nil)
	if err != nil {
		return
	}
	return data, nil
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

// UpdateProjectStatus update project status to given value and update update_ts field
func (*projectModel) UpdateProjectStatus(db *gorm.DB, projectId uint, status ProjectStatus) error {
	if err := db.Where(&Project{ID: projectId}).Updates(&Project{
		Status:   status,
		UpdateTs: GetCurrentUtcEpochSecond(),
	}).Error; err != nil {
		log.Error().Msgf("Update project %d status to open error: %+v", projectId, err)
		return err
	}
	return nil
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
		Name:        internal.CityHallProjectName,
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

func (*projectBudgetModel) WithdrawSingleAsset(db *gorm.DB, projID uint, assetName string, amount decimal.Decimal) error {
	return db.Transaction(func(tx *gorm.DB) error {
		budgetRcd, err := ProjectBudgetModel.QueryByProjectIdAndBudgetProps(tx, projID, assetName)
		if err != nil {
			log.Error().Msgf("query project %d budget %s error: %+v", projID, assetName, err)
			return err
		}

		updateClause := map[string]any{
			"id": budgetRcd.ID,
		}

		if budgetRcd.RemainAmount.LessThan(amount) {
			err = fmt.Errorf("project %d budget %s remain amount %s is less than request value %s", projID, assetName, budgetRcd.RemainAdvanceAmount.String(), amount.String())
			log.Error().Msgf(err.Error())
			return err
		} else {
			updateClause["used_amount"] = budgetRcd.UsedAmount.Add(amount)
			updateClause["remain_amount"] = budgetRcd.RemainAmount.Sub(amount)
		}

		// Handle budget record with advance ratio
		if !budgetRcd.AdvanceRatio.Equal(decimal.Zero) {
			if budgetRcd.RemainAdvanceAmount.LessThan(amount) {
				err = fmt.Errorf("project %d budget %s remain advance amount %s is less than request value %s", projID, assetName, budgetRcd.RemainAdvanceAmount.String(), amount.String())
				log.Error().Msgf(err.Error())
				return err
			} else {
				updateClause["used_advance_amount"] = budgetRcd.UsedAdvanceAmount.Add(amount)
				updateClause["remain_advance_amount"] = budgetRcd.RemainAdvanceAmount.Sub(amount)
			}
		}

		return tx.Model(&budgetRcd).Updates(updateClause).Error
	})
}
func (*projectBudgetModel) DepositSingleAsset(db *gorm.DB, projID uint, assetName string, amount decimal.Decimal) error {
	return db.Transaction(func(tx *gorm.DB) error {
		budgetRcd, err := ProjectBudgetModel.QueryByProjectIdAndBudgetProps(tx, projID, assetName)
		if err != nil {
			log.Error().Msgf("query project %d budget %s error: %+v", projID, assetName, err)
			return err
		}

		updateClause := map[string]any{
			"id": budgetRcd.ID,
		}

		if budgetRcd.UsedAmount.LessThan(amount) {
			err = fmt.Errorf("project %d budget %s used amount %s is less than request value %s", projID, assetName, budgetRcd.UsedAmount.String(), amount.String())
			log.Error().Msgf(err.Error())
			return err
		} else {
			updateClause["used_amount"] = budgetRcd.UsedAmount.Sub(amount)
			updateClause["remain_amount"] = budgetRcd.RemainAmount.Add(amount)
		}

		// Handle budget record with advance ratio
		if !budgetRcd.AdvanceRatio.Equal(decimal.Zero) {
			if budgetRcd.UsedAdvanceAmount.LessThan(amount) {
				err = fmt.Errorf("project %d budget %s used advance amount %s is less than request value %s", projID, assetName, budgetRcd.UsedAdvanceAmount.String(), amount.String())
				log.Error().Msgf(err.Error())
				return err
			} else {
				updateClause["used_advance_amount"] = budgetRcd.UsedAdvanceAmount.Sub(amount)
				updateClause["remain_advance_amount"] = budgetRcd.RemainAdvanceAmount.Add(amount)
			}
		}

		return tx.Model(&budgetRcd).Updates(updateClause).Error
	})
}
