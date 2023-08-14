package city_hall

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

type (
	CityHallDetailReply struct {
		model.Project
		Budgets []*model.ProjectBudget `json:"budgets"`
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
