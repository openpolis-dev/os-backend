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
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
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

	for grpName, _ := range cityHallProject.GroupedSponsors {
		if _, found := internal.CityhallGroupNames[grpName]; !found {
			delete(cityHallProject.GroupedSponsors, grpName)
		}
	}

	return cityHallProject, err
}

// Info returns cityhall info
//
//	@summary	Return cityhall info
//	@route		/cityhall/info [get]
//
//	@success	200	{object}	CityHallDetailReply
func Info(ctx *gin.Context) {
	_, enforcer, db, _ := api.ForContext(ctx)
	cityHallProject, err := getOrCreateCityHallProject(db, enforcer)
	cityHallProject.GroupedSponsors = lo.PickBy(cityHallProject.GroupedSponsors, func(_ string, members []string) bool {
		return members != nil && len(members) != 0
	})

	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall record error")))
		return
	}

	budgets, err := model.ProjectBudgetModel.ListByProjectId(db, cityHallProject.ID)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(generateCityHallDetailReply(cityHallProject, budgets)))
}

// UpdateBudget updates cityhall budget for current season
//
//	@summary	updates cityhall budget for current season
//	@route		/cityhall/update_budget [post]
//
//	@param		JsonBody	body		CityHallUpdateBudgetReq	true	"update budget request"
//
//	@success	200			{string}	nil
func UpdateBudget(ctx *gin.Context) {
	user, enforcer, db, _ := api.ForContext(ctx)
	formattedWallet := common.FormatUserWallet(user.Wallet)
	cityHallProject, err := getOrCreateCityHallProject(db, enforcer)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall record error")))
		return
	}

	req := CityHallUpdateBudgetReq{}
	err = ctx.BindJSON(&req)
	if err != nil {
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	//  check permission
	ok, err := enforcer.HasRoleForUser(formattedWallet, api.RoleHall)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, api.RoleHall, "access")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	budget := model.ProjectBudget{}
	if req.AssetName == "" || req.AssetType == "" || req.TotalAmount == decimal.Zero {
		err := errors.New("all fields in request should be filled")
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
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
				CreatedAt:    time.Now().In(internal.ProjectTimezone),
				UpdatedAt:    time.Now().In(internal.ProjectTimezone),
				CreateTs:     model.GetCurrentUtcEpochSecond(),
				UpdateTs:     model.GetCurrentUtcEpochSecond(),
			}
			err = db.Create(&budget).Error
			if err != nil {
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
				return
			}
		} else {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
	}

	// update `TotalAmount`
	budget.TotalAmount = req.TotalAmount
	budget.RemainAmount = budget.TotalAmount.Sub(budget.UsedAmount)
	budget.UpdatedAt = time.Now().In(internal.ProjectTimezone)
	budget.UpdateTs = model.GetCurrentUtcEpochSecond()
	err = model.ProjectBudgetModel.Update(db, &budget)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// UpdateMember updates cityhall member, if group name existing in the request, the grouped sponsors field will be updated, otherwise the sponsors field will be updated
//
//	@summary	updates cityhall member, if group name existing in the request, the grouped sponsors field will be updated, otherwise the sponsors field will be updated
//	@router		/cityhall/update_members [post]
//	@param		JsonBody	body		CityHallUpdateMemberReq	true	"member data"
//	@success	200			{string}	nil
func UpdateMember(ctx *gin.Context) {
	user, enforcer, db, _ := api.ForContext(ctx)
	formattedWallet := common.FormatUserWallet(user.Wallet)
	log.Debug().Msgf("update city hall request form user %s", formattedWallet)
	cityHallProject, err := getOrCreateCityHallProject(db, enforcer)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall record error")))
		return
	}

	//  check permission
	ok, err := enforcer.HasRoleForUser(formattedWallet, api.RoleHall)
	if err != nil {
		log.Error().Msgf("check permission error %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	if !ok {
		log.Warn().Msgf("permission deny for user %s", formattedWallet)
		sdk.LogForbiddenError(ctx, user.Wallet, api.RoleHall, "access")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	req := CityHallUpdateMemberReq{}
	err = ctx.BindJSON(&req)
	if err != nil {
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}
	log.Debug().Msgf("city hall update member form user %s", formattedWallet)

	//if _, found := internal.CityhallGroupNames[req.GroupName]; !found {
	//	ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("invalid group_name %s", req.GroupName)))
	//	return
	//}

	statusCode, err := updateGroupedMembers(cityHallProject, &req, db, enforcer)
	switch statusCode {
	case http.StatusBadRequest:
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	case http.StatusInternalServerError:
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	case http.StatusOK:
		budgets, err := model.ProjectBudgetModel.ListByProjectId(db, cityHallProject.ID)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}

		ctx.JSON(http.StatusOK, api.Success(generateCityHallDetailReply(cityHallProject, budgets)))
		return
	}
}

// BatchUpdateMembers updates multiple group member info in single request, the logic is same with single update
//
//	@summary	updates multiple group member info in single request, the logic is same with single update
//	@router		/cityhall/batch_update_members [post]
//	@param		JsonBody	body		[]CityHallUpdateMemberReq	true	"member data"
//	@success	200			{string}	nil
func BatchUpdateMembers(ctx *gin.Context) {
	user, enforcer, db, _ := api.ForContext(ctx)
	formattedWallet := common.FormatUserWallet(user.Wallet)
	log.Debug().Msgf("update city hall request form user %s", formattedWallet)
	cityHallProject, err := getOrCreateCityHallProject(db, enforcer)
	if err != nil {
		log.Error().Msgf("get cityhall record error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall record error")))
		return
	}

	//  check permission
	ok, err := enforcer.HasRoleForUser(formattedWallet, api.RoleHall)
	if err != nil {
		log.Error().Msgf("check permission error %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	if !ok {
		log.Warn().Msgf("permission deny for user %s", formattedWallet)
		sdk.LogForbiddenError(ctx, user.Wallet, api.RoleHall, "access")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	var req []*CityHallUpdateMemberReq
	err = ctx.BindJSON(&req)
	if err != nil {
		log.Error().Msgf("parse body params error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse body params error: %+v", err)))
		return
	}
	log.Debug().Msgf("city hall update member form user %s, request: %+v", formattedWallet, req)

	for _, updateMemberReq := range req {
		statusCode, err := updateGroupedMembers(cityHallProject, updateMemberReq, db, enforcer)
		if err != nil {
			switch statusCode {
			case http.StatusBadRequest:
				sdk.LogUserSideError(ctx, err)
				ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
				return
			case http.StatusInternalServerError:
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
				return
			}
		}
	}
	budgets, err := model.ProjectBudgetModel.ListByProjectId(db, cityHallProject.ID)
	if err != nil {
		log.Error().Msgf("get project budget error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(generateCityHallDetailReply(cityHallProject, budgets)))
}

// updateGroupedMembers is used to parse CityHallUpdateMemberReq data and update city hall members
func updateGroupedMembers(cityHallProject *model.Project, req *CityHallUpdateMemberReq, db *gorm.DB, enforcer *casbin.Enforcer) (int, error) {
	// TODO: All checking groupName is "" is workaround logic for non grouped request, will be changed to grouped version after FE updated
	if req.GroupName != "" {
		if _, found := internal.CityhallGroupNames[req.GroupName]; !found {
			return http.StatusBadRequest, fmt.Errorf("invalid group_name %s", req.GroupName)
		}
	}

	sponsorsMap := make(map[string]bool)

	// TODO: All checking groupName is "" is workaround logic for non grouped request, will be changed to grouped version after FE updated
	if req.GroupName != "" {
		if sponsors, found := cityHallProject.GroupedSponsors[req.GroupName]; found {
			for _, sponsorWallet := range sponsors {
				sponsorsMap[common.FormatUserWallet(sponsorWallet)] = true
			}
		}
	} else {
		for _, sponsorWallet := range cityHallProject.Sponsors {
			sponsorsMap[common.FormatUserWallet(sponsorWallet)] = true
		}
	}

	///////////////////////////////
	// Update grouping policy
	///////////////////////////////

	// Add member to policy group
	var addHallGroupingPolicy [][]string
	for _, memberAddr := range req.AddMember {
		sponsorsMap[common.FormatUserWallet(memberAddr)] = true
		addHallGroupingPolicy = append(addHallGroupingPolicy, []string{common.FormatUserWallet(memberAddr), api.RoleHall})
	}

	// Add user to hall group
	if len(addHallGroupingPolicy) > 0 {
		log.Debug().Msgf("add hall group policy: %+v", addHallGroupingPolicy)
		_, err := enforcer.AddGroupingPolicies(addHallGroupingPolicy)
		if err != nil {
			log.Error().Msgf("add hall grouping policy error %+v, policy: %+v", err, addHallGroupingPolicy)
			return http.StatusInternalServerError, err
		}
	}

	// Remove member from policy group
	var removeHallGroupingPolicy [][]string
	for _, memberAddr := range req.RemoveMember {
		sponsorsMap[common.FormatUserWallet(memberAddr)] = false
		removeHallGroupingPolicy = append(removeHallGroupingPolicy, []string{common.FormatUserWallet(memberAddr), api.RoleHall})
	}

	// Remove user from hall group
	if len(removeHallGroupingPolicy) > 0 {
		log.Debug().Msgf("remove hall group policy: %+v", removeHallGroupingPolicy)
		_, err := enforcer.RemoveGroupingPolicies(removeHallGroupingPolicy)
		if err != nil {
			log.Error().Msgf("remove hall grouping policy error %+v, policy: %+v", err, removeHallGroupingPolicy)
			return http.StatusInternalServerError, err
		}
	}

	// Save policy
	err := enforcer.SavePolicy()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	// Update sponsors record in DB
	// TODO: All checking groupName is "" is workaround logic for non grouped request, will be changed to grouped version after FE updated
	if req.GroupName != "" {
		var newSponsorsList []string
		for memberAddr, confirmedSponsors := range sponsorsMap {
			if confirmedSponsors {
				newSponsorsList = append(newSponsorsList, memberAddr)
			}
		}

		if cityHallProject.GroupedSponsors == nil {
			cityHallProject.GroupedSponsors = make(map[string][]string)
		}

		cityHallProject.GroupedSponsors[req.GroupName] = newSponsorsList
	} else {
		var newSponsorsList []string
		for memberAddr, confirmedSponsors := range sponsorsMap {
			if confirmedSponsors {
				newSponsorsList = append(newSponsorsList, memberAddr)
			}
		}
		cityHallProject.Sponsors = newSponsorsList
	}

	cityHallProject.UpdatedAt = time.Now().In(internal.ProjectTimezone)
	cityHallProject.UpdateTs = model.GetCurrentUtcEpochSecond()
	err = db.Save(cityHallProject).Error
	if err != nil {
		return http.StatusInternalServerError, err
	}

	return http.StatusOK, nil
}

func generateCityHallDetailReply(cityHallProject *model.Project, budgets []*model.ProjectBudget) *CityHallDetailReply {
	cityHallProject.Members = lo.Map(cityHallProject.Members, func(m string, _ int) string {
		return common.ToFrontendWallet(m)
	})
	cityHallProject.Sponsors = lo.Map(cityHallProject.Sponsors, func(m string, _ int) string {
		return common.ToFrontendWallet(m)
	})

	for grpName, wallets := range cityHallProject.GroupedSponsors {
		cityHallProject.GroupedSponsors[grpName] = lo.Map(wallets, func(m string, _ int) string {
			return common.ToFrontendWallet(m)
		})
	}

	return &CityHallDetailReply{
		Project: *cityHallProject,
		Budgets: budgets,
	}
}
