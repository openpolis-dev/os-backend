package city_hall

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

type (
	CityHallDetailReply struct {
		model.Project
		Budgets []*model.ProjectBudget `json:"budgets"`
	}
	CityHallUpdateBudgetReq struct {
		AssetType   string          `json:"asset_type"`
		AssetName   string          `json:"asset_name"`
		TotalAmount decimal.Decimal `json:"total_amount"`
	}
)

func createCityHallProject(db *gorm.DB, cityHallUsers []string) (*model.Project, error) {
	project := model.Project{
		Name:        "CityHall",
		IsSpecial:   true,
		SpecialType: model.SpecialProjectCityHall,
		Sponsors:    cityHallUsers,
		CreatedAt:   time.Time{},
		UpdatedAt:   time.Time{},
	}
	err := db.Create(&project).Error

	if err != nil {
		return nil, err
	}
	return &project, nil
}

func getCityHallProject(db *gorm.DB, cityHallUsers []string) (*model.Project, error) {
	project := model.Project{}
	db.Where(model.Project{
		IsSpecial:   true,
		SpecialType: model.SpecialProjectCityHall,
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

func Info(ctx *gin.Context) {
	_, enforcer, db, _ := api.ForContext(ctx)
	configuredCityHallUser, err := enforcer.GetUsersForRole(api.RoleHall)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error")))
		return
	}

	project, err := getCityHallProject(db, configuredCityHallUser)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall record error")))
		return
	}

	budgets, err := model.ProjectBudgetModel.ListByProjectId(db, project.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&CityHallDetailReply{
		Project: *project,
		Budgets: budgets,
	}))
}

func UpdateBudget(ctx *gin.Context) {
	user, enforcer, db, _ := api.ForContext(ctx)
	configuredCityHallUser, err := enforcer.GetUsersForRole(api.RoleHall)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error")))
		return
	}

	cityHallProject, err := getCityHallProject(db, configuredCityHallUser)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall record error")))
		return
	}

	req := CityHallUpdateBudgetReq{}
	err = ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	//  check permission
	ok, err := enforcer.HasRoleForUser(user.Wallet, api.RoleHall)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	if !ok {
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	budget := model.ProjectBudget{}
	err = db.Where(&model.ProjectBudget{
		ProjectID: cityHallProject.ID,
		AssetName: req.AssetName,
		Type:      model.BudgetType(req.AssetType),
	}).First(&budget).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			budget = model.ProjectBudget{
				ProjectID:    cityHallProject.ID,
				AssetName:    req.AssetName,
				Type:         model.BudgetType(req.AssetType),
				TotalAmount:  req.TotalAmount,
				UsedAmount:   decimal.Zero,
				RemainAmount: req.TotalAmount,
				CreatedAt:    time.Time{},
				UpdatedAt:    time.Time{},
			}
			err = db.Create(&budget).Error
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
				return
			}
		} else {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
	}

	// update `TotalAmount`
	budget.TotalAmount = req.TotalAmount
	budget.RemainAmount = budget.TotalAmount.Sub(budget.UsedAmount)
	err = model.ProjectBudgetModel.Update(db, &budget)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func UpdateMember(ctx *gin.Context) {}
