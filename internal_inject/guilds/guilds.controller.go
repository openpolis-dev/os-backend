package guilds_inject

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type GuildsController struct {
	// inject

	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	GuildesService *GuildsService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var guilds GuildsController

	err := inject.Populate(&guilds, &GuildsService{}, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var guildsGroup *gin.RouterGroup
	var guildsAuthGroup *gin.RouterGroup

	if fatherGroup != nil {
		guildsGroup = fatherGroup.Group("/guilds")
		guildsAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/guilds")
	} else {
		guildsGroup = guilds.Gin.Group("/guilds")
		guildsAuthGroup = guilds.Gin.Group("/", middleware.AuthRequired).Group("/guilds")
	}

	// no auth
	guildsGroup.GET("/", guilds.List)
	guildsGroup.GET("/:id", guilds.Detail)
	guildsGroup.GET("/:id/budgets", guilds.ShowBudgets)

	// auth
	guildsAuthGroup.POST("/", guilds.Create)
	guildsAuthGroup.PUT("/:id", guilds.Update)
	guildsAuthGroup.POST("/:id/update_staffs", guilds.UpdateStaffs)
	guildsAuthGroup.POST("/:id/update_budget", guilds.UpdateBudget)
	guildsAuthGroup.POST("/:id/add_related_proposal", guilds.AddRelatedProposal)
	guildsAuthGroup.POST("/:id/close", guilds.Close)
	// my guilds
	if fatherGroup != nil {
		fatherGroup.Group("/", middleware.AuthRequired).GET("/my_guilds", guilds.MyGuilds)
	} else {
		guilds.Gin.Group("/", middleware.AuthRequired).GET("/my_guilds", guilds.MyGuilds)
	}
}

func (c *GuildsController) List(ctx *gin.Context) {
	page := api.ParseAndConvertPageParam(ctx)

	keywords := ctx.Query("keywords")
	var k *string
	if keywords != "" {
		k = &keywords
	}

	wallet := ctx.Query("wallet")
	var w *string
	if wallet != "" {
		w = &wallet
	}

	guilds, total, err := model.GuildModel.ListWithSearch(c.Db, k, w, page)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list guilds error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&api.ListReplyData{
		Page:  page.Page,
		Size:  page.Size,
		Total: total,
		Rows: lo.Map(guilds, func(m *model.Guild, _ int) *model.Guild {
			return NormalizeWalletAddrInGuild(m)
		}),
	}))
}

func (c *GuildsController) Detail(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	db := api.ForContextOnlyDB(ctx)

	guild, err := model.GuildModel.Detail(db, uint(id))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get guild error")))
		return
	}
	if guild == nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("guild %d not exist", id)))
		return
	}

	budgets, err := model.GuildBudgetModel.ListByGuildId(db, guild.ID)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get guild budgets error")))
		return
	}

	guild.Members = lo.Map(guild.Members, func(m string, _ int) string {
		return common.ToFrontendWallet(m)
	})

	guild.Sponsors = lo.Map(guild.Sponsors, func(m string, _ int) string {
		return common.ToFrontendWallet(m)
	})

	ctx.JSON(http.StatusOK, api.Success(&DetailReply{
		Guild:   *NormalizeWalletAddrInGuild(guild),
		Budgets: budgets,
	}))
}

func (c *GuildsController) ShowBudgets(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	db := api.ForContextOnlyDB(ctx)

	budgets, err := model.GuildBudgetModel.ListByGuildId(db, uint(id))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project budgets error")))
		return
	}

	budgetResp := GenerateGuildBudgetResp(budgets)
	ctx.JSON(http.StatusOK, api.Success(budgetResp))
}

func (c *GuildsController) Create(ctx *gin.Context) {
	req := CreateReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	httpCode, reply := c.GuildesService.Create(ctx, &req)

	ctx.JSON(httpCode, reply)
}

func (c *GuildsController) Update(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	req := UpdateReq{}
	err = ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	httpCode, reply := c.GuildesService.Update(ctx, id, &req)

	ctx.JSON(httpCode, reply)
}

func (c *GuildsController) UpdateStaffs(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	req := UpdateStaffsReq{}
	err = ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	httpCode, reply := c.GuildesService.UpdateStaffs(ctx, id, &req)

	ctx.JSON(httpCode, reply)
}

func (c *GuildsController) UpdateBudget(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	req := UpdateBudgetReq{}
	err = ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	httpCode, reply := c.GuildesService.UpdateBudget(ctx, id, &req)

	ctx.JSON(httpCode, reply)
}

func (c *GuildsController) AddRelatedProposal(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}
	proposalIDs := ctx.QueryArray("proposalIDs")

	httpCode, reply := c.GuildesService.AddRelatedProposal(ctx, id, proposalIDs)

	ctx.JSON(httpCode, reply)
}

func (c *GuildsController) Close(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	httpCode, reply := c.GuildesService.Close(ctx, id)

	ctx.JSON(httpCode, reply)
}

func (c *GuildsController) MyGuilds(ctx *gin.Context) {
	user := api.ForContextOnlyUser(ctx)

	page := api.ParseAndConvertPageParam(ctx)

	guilds, total, err := model.GuildModel.ListBySponsorOrMember(c.Db, common.FormatUserWallet(user.Wallet), page)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list guilds error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&api.ListReplyData{
		Page:  page.Page,
		Size:  page.Size,
		Total: total,
		Rows: lo.Map(guilds, func(m *model.Guild, _ int) *model.Guild {
			return NormalizeWalletAddrInGuild(m)
		}),
	}))
}
