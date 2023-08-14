package city_hall

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
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

	CityHallUpdateMemberReq struct {
		AddMember    []string `json:"add"`
		RemoveMember []string `json:"remove"`
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

func getOrCreateCityHallProject(db *gorm.DB, enforcer *casbin.Enforcer) (*model.Project, error) {
	configuredCityHallUser, err := enforcer.GetUsersForRole(api.RoleHall)
	if err != nil {
		return nil, errors.New("get cityhall permission error")
	}

	cityHallProject, err := getCityHallProject(db, configuredCityHallUser)
	if err != nil {
		return nil, errors.New("get cityhall record error")
	}

	return cityHallProject, err
}

func Info(ctx *gin.Context) {
	_, enforcer, db, _ := api.ForContext(ctx)
	cityHallProject, err := getOrCreateCityHallProject(db, enforcer)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall record error")))
		return
	}

	budgets, err := model.ProjectBudgetModel.ListByProjectId(db, cityHallProject.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&CityHallDetailReply{
		Project: *cityHallProject,
		Budgets: budgets,
	}))
}

func UpdateBudget(ctx *gin.Context) {
	user, enforcer, db, _ := api.ForContext(ctx)
	cityHallProject, err := getOrCreateCityHallProject(db, enforcer)
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
	if req.AssetName == "" || req.AssetType == "" || req.TotalAmount == decimal.Zero {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("all fields in request should be filled")))
		return
	}
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

func UpdateMember(ctx *gin.Context) {
	user, enforcer, db, _ := api.ForContext(ctx)
	cityHallProject, err := getOrCreateCityHallProject(db, enforcer)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall record error")))
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

	req := CityHallUpdateMemberReq{}
	err = ctx.BindJSON(&req)

	sponsorsMap := make(map[string]bool)
	for _, userAddr := range cityHallProject.Sponsors {
		sponsorsMap[userAddr] = true
	}

	for _, memberAddr := range req.AddMember {
		sponsorsMap[memberAddr] = true
	}

	for _, memberAddr := range req.RemoveMember {
		sponsorsMap[memberAddr] = false
	}

	newSponsorsList := []string{}
	for memberAddr, confirmedSponsors := range sponsorsMap {
		if confirmedSponsors {
			newSponsorsList = append(newSponsorsList, memberAddr)
		}
	}

	groupPolicies := lo.Map[string, []string](newSponsorsList, func(user string, _ int) []string {
		return []string{strings.ToLower(user), api.RoleHall}
	})
	_, err = enforcer.AddGroupingPolicies(groupPolicies) // update hall users
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	err = enforcer.SavePolicy()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	cityHallProject.Sponsors = newSponsorsList
	err = db.Save(cityHallProject).Error
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	budgets, err := model.ProjectBudgetModel.ListByProjectId(db, cityHallProject.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&CityHallDetailReply{
		Project: *cityHallProject,
		Budgets: budgets,
	}))
}
