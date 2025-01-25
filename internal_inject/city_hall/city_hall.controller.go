package cityhall_inject

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
)

type CityHallController struct {
	// inject

	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	CityHallService *CityHallService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var cityHall CityHallController

	err := inject.Populate(&cityHall, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var cityHallGroup *gin.RouterGroup
	var cityHallAuthGroup *gin.RouterGroup

	if fatherGroup != nil {
		cityHallGroup = fatherGroup.Group("/cityhall")
		cityHallAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/cityhall")
	} else {
		cityHallGroup = cityHall.Gin.Group("/cityhall")
		cityHallAuthGroup = cityHall.Gin.Group("/", middleware.AuthRequired).Group("/cityhall")
	}

	// no auth
	cityHallGroup.GET("/info", cityHall.Info)
	cityHallGroup.GET("/cs_node", cityHall.CurrentSeasonNodeList)

	// auth
	cityHallAuthGroup.POST("/update_budget", cityHall.UpdateBudget)
	cityHallAuthGroup.POST("/update_members", cityHall.UpdateMember)
	cityHallAuthGroup.POST("/batch_update_members", cityHall.BatchUpdateMembers)
}

func (c *CityHallController) Info(ctx *gin.Context) {
	_, enforcer, _, _ := api.ForContext(ctx)
	cityHallProject, err := c.CityHallService.GetOrCreateCityHallProject(c.Db, enforcer)
	cityHallProject.GroupedSponsors = lo.PickBy(cityHallProject.GroupedSponsors, func(_ string, members []string) bool {
		return len(members) != 0
	})

	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall record error")))
		return
	}

	budgets, err := model.ProjectBudgetModel.ListByProjectId(c.Db, cityHallProject.ID)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall budget error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(c.CityHallService.GenerateCityHallDetailReply(cityHallProject, budgets)))

}

func (c *CityHallController) CurrentSeasonNodeList(ctx *gin.Context) {
	tokenAddr, tokenId, err := model.GetNodeSbtAddrAndId(c.Db)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get node sbt address and id error")))
		return
	}

	cachedData, err := storage.GetCachedData(storage.CurrentSeasonNodeCacheKey(tokenAddr, tokenId))
	var csNodeWallets []string
	if err != nil {
		log.Warn().Msgf("get cached data error: %+v, try to fetch from indexer", err)
		// TODO: Fetch node list from indexer
		log.Debug().Msgf("node sbt address: %s, node sbt id: %s", tokenAddr, tokenId)

		currSeason, err := model.GetCurrentSeason(c.Db)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get current season error")))
			return
		}
		indexerClient := sdk.GetIndexerClient()
		csNodeWallets, err = indexerClient.GetCurrentSeasonNodeList(fmt.Sprintf("%d", currSeason.Idx))

		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get current season node list error")))
			return
		}

		csNodeBytes, err := json.Marshal(csNodeWallets)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("marshal current season node list error")))
			return
		}
		_ = storage.StoreCachedData(storage.CurrentSeasonNodeCacheKey(tokenAddr, tokenId), csNodeBytes)
	} else {
		err = json.Unmarshal(cachedData, &csNodeWallets)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("unmarshal current season node list error")))
			return
		}
	}

	ctx.JSON(http.StatusOK, api.Success(csNodeWallets))
}

func (c *CityHallController) UpdateBudget(ctx *gin.Context) {
	user, enforcer, _, _ := api.ForContext(ctx)
	formattedWallet := common.FormatUserWallet(user.Wallet)
	cityHallProject, err := c.CityHallService.GetOrCreateCityHallProject(c.Db, enforcer)
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
	ok, err := enforcer.HasRoleForUser(formattedWallet, internal.RoleHall)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("get cityhall permission error detail:"+err.Error())))
		return
	}

	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
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
	err = c.Db.Where(&model.ProjectBudget{
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
			err = c.Db.Create(&budget).Error
			if err != nil {
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create cityhall budget error detail:"+err.Error())))
				return
			}
		} else {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall budget error")))
			return
		}
	}

	// update `TotalAmount`
	budget.TotalAmount = req.TotalAmount
	budget.RemainAmount = budget.TotalAmount.Sub(budget.UsedAmount)
	budget.UpdatedAt = time.Now().In(internal.ProjectTimezone)
	budget.UpdateTs = model.GetCurrentUtcEpochSecond()
	err = model.ProjectBudgetModel.Update(c.Db, &budget)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update cityhall budget error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func (c *CityHallController) UpdateMember(ctx *gin.Context) {
	user, enforcer, _, _ := api.ForContext(ctx)
	formattedWallet := common.FormatUserWallet(user.Wallet)
	log.Debug().Msgf("update city hall request form user %s", formattedWallet)
	cityHallProject, err := c.CityHallService.GetOrCreateCityHallProject(c.Db, enforcer)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall record error")))
		return
	}

	//  check permission
	ok, err := enforcer.HasRoleForUser(formattedWallet, internal.RoleHall)
	if err != nil {
		log.Error().Msgf("check permission error %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("get cityhall permission error detail:"+err.Error())))
		return
	}

	if !ok {
		log.Warn().Msgf("permission deny for user %s", formattedWallet)
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
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

	statusCode, err := c.CityHallService.UpdateGroupedMembers(cityHallProject, &req, c.Db, enforcer)
	switch statusCode {
	case http.StatusBadRequest:
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	case http.StatusInternalServerError:
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update cityhall member error detail:"+err.Error())))
		return
	case http.StatusOK:
		budgets, err := model.ProjectBudgetModel.ListByProjectId(c.Db, cityHallProject.ID)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall budget error")))
			return
		}

		ctx.JSON(http.StatusOK, api.Success(c.CityHallService.GenerateCityHallDetailReply(cityHallProject, budgets)))
		return
	}
}

func (c *CityHallController) BatchUpdateMembers(ctx *gin.Context) {
	user, enforcer, db, _ := api.ForContext(ctx)
	formattedWallet := common.FormatUserWallet(user.Wallet)
	log.Debug().Msgf("update city hall request form user %s", formattedWallet)
	cityHallProject, err := c.CityHallService.GetOrCreateCityHallProject(db, enforcer)
	if err != nil {
		log.Error().Msgf("get cityhall record error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall record error")))
		return
	}

	//  check permission
	ok, err := enforcer.HasRoleForUser(formattedWallet, internal.RoleHall)
	if err != nil {
		log.Error().Msgf("check permission error %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("get cityhall permission error detail:"+err.Error())))
		return
	}

	if !ok {
		log.Warn().Msgf("permission deny for user %s", formattedWallet)
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
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
		statusCode, err := c.CityHallService.UpdateGroupedMembers(cityHallProject, updateMemberReq, db, enforcer)
		if err != nil {
			switch statusCode {
			case http.StatusBadRequest:
				sdk.LogUserSideError(ctx, err)
				ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
				return
			case http.StatusInternalServerError:
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update cityhall member error detail:"+err.Error())))
				return
			}
		}
	}
	budgets, err := model.ProjectBudgetModel.ListByProjectId(db, cityHallProject.ID)
	if err != nil {
		log.Error().Msgf("get project budget error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall budget error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(c.CityHallService.GenerateCityHallDetailReply(cityHallProject, budgets)))

}
