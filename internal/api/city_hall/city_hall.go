package city_hall

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
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
		GroupName    string   `json:"group_name"`
	}
)

// getOrCreateCityHallProject validate user's permission and then get or create cityhall project in DB
func getOrCreateCityHallProject(db *gorm.DB, enforcer *casbin.Enforcer) (*model.Project, error) {
	configuredCityHallUser, err := enforcer.GetUsersForRole(api.RoleHall)
	if err != nil {
		return nil, errors.New("get cityhall permission error")
	}

	cityHallProject, err := model.GetOrCreateCityHallProject(db, configuredCityHallUser)
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
	}).First(&budget).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			budget = model.ProjectBudget{
				ProjectID:    cityHallProject.ID,
				AssetName:    req.AssetName,
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
	log.Debug().Msgf("update city hall request form user %s", user.Wallet)
	cityHallProject, err := getOrCreateCityHallProject(db, enforcer)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall record error")))
		return
	}

	//  check permission
	ok, err := enforcer.HasRoleForUser(user.Wallet, api.RoleHall)
	if err != nil {
		log.Error().Msgf("check permission error %+v", err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	if !ok {
		log.Warn().Msgf("permission deny for user %s", user.Wallet)
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	req := CityHallUpdateMemberReq{}
	err = ctx.BindJSON(&req)
	log.Debug().Msgf("city hall update member form user %s", user.Wallet)

	if !lo.Contains(internal.CityhallGroupNames, req.GroupName) {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("invalid group_name %s", req.GroupName)))
		return
	}

	sponsorsMap := make(map[string]bool)
	if sponsors, found := cityHallProject.GroupedSponsors[req.GroupName]; found {
		for _, sponsorWallet := range sponsors {
			sponsorsMap[model.FormatUserWallet(sponsorWallet)] = true
		}
	}

	///////////////////////////////
	// Update grouping policy
	///////////////////////////////

	// Add member to policy group
	var addHallGroupingPolicy [][]string
	for _, memberAddr := range req.AddMember {
		sponsorsMap[model.FormatUserWallet(memberAddr)] = true
		addHallGroupingPolicy = append(addHallGroupingPolicy, []string{model.FormatUserWallet(memberAddr), api.RoleHall})
	}

	// Add user to hall group
	if len(addHallGroupingPolicy) > 0 {
		log.Debug().Msgf("add hall group policy: %+v", addHallGroupingPolicy)
		_, err = enforcer.AddGroupingPolicies(addHallGroupingPolicy)
		if err != nil {
			log.Error().Msgf("add hall grouping policy error %+v, policy: %+v", err, addHallGroupingPolicy)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
	}

	// Remove member from policy group
	var removeHallGroupingPolicy [][]string
	for _, memberAddr := range req.RemoveMember {
		sponsorsMap[model.FormatUserWallet(memberAddr)] = false
		removeHallGroupingPolicy = append(removeHallGroupingPolicy, []string{model.FormatUserWallet(memberAddr), api.RoleHall})
	}

	// Remove user from hall group
	if len(removeHallGroupingPolicy) > 0 {
		log.Debug().Msgf("remove hall group policy: %+v", removeHallGroupingPolicy)
		_, err = enforcer.RemoveGroupingPolicies(removeHallGroupingPolicy)
		if err != nil {
			log.Error().Msgf("remove hall grouping policy error %+v, policy: %+v", err, removeHallGroupingPolicy)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
	}

	// Save policy
	err = enforcer.SavePolicy()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	// Update sponsors record in DB
	var newSponsorsList []string
	for memberAddr, confirmedSponsors := range sponsorsMap {
		if confirmedSponsors {
			newSponsorsList = append(newSponsorsList, memberAddr)
		}
	}

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	if cityHallProject.GroupedSponsors == nil {
		cityHallProject.GroupedSponsors = make(map[string][]string)
	}

	cityHallProject.GroupedSponsors[req.GroupName] = newSponsorsList
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
