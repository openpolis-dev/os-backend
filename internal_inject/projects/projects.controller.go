package projects_inject

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
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

type ProjectsController struct {
	// inject

	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	ProjectsService *ProjectsService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var projects ProjectsController

	err := inject.Populate(&projects, &ProjectsService{}, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var projectsGroup *gin.RouterGroup
	var projectsAuthGroup *gin.RouterGroup

	if fatherGroup != nil {
		projectsGroup = fatherGroup.Group("/projects")
		projectsAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/projects")
	} else {
		projectsGroup = projects.Gin.Group("/projects")
		projectsAuthGroup = projects.Gin.Group("/", middleware.AuthRequired).Group("/projects")
	}

	// no auth
	projectsGroup.GET("/", projects.List)
	projectsGroup.GET("/:id", projects.Detail)
	projectsGroup.GET("/:id/budgets", projects.ShowBudgets)

	// auth
	projectsAuthGroup.POST("/", projects.Create)
	projectsAuthGroup.PUT("/:id", projects.Update)
	projectsAuthGroup.POST("/:id/close", projects.Close)
	projectsAuthGroup.POST("/:id/update_staffs", projects.UpdateStaffs)
	projectsAuthGroup.POST("/:id/update_budget", projects.UpdateBudget)
	projectsAuthGroup.POST("/:id/add_related_proposal", projects.AddRelatedProposal)
	// my projects
	if fatherGroup != nil {
		fatherGroup.Group("/", middleware.AuthRequired).GET("/my_projects", projects.MyProjects)
	} else {
		projects.Gin.Group("/", middleware.AuthRequired).GET("/my_projects", projects.MyProjects)
	}
}

func (c *ProjectsController) List(ctx *gin.Context) {
	status := ctx.Query("status")
	page := api.ParseAndConvertPageParam(ctx)

	showSpecialProjectsParam := ctx.Query("show_special")
	showSpecialProjectFlag := strings.EqualFold(showSpecialProjectsParam, "true")

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

	projects, total, err := model.ProjectModel.ListWithSearch(c.Db, status, k, w, page, showSpecialProjectFlag)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list projects error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  page.Page,
		Size:  page.Size,
		Total: total,
		Rows: lo.Map(projects, func(project *model.Project, _ int) model.Project {
			return *NormalizeWalletAddrInProject(project)
		}),
	}))
}

func (c *ProjectsController) Detail(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	proj, err := model.ProjectModel.Detail(c.Db, uint(id))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project error")))
		return
	}
	if proj == nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id)))
		return
	}

	budgets, err := model.ProjectBudgetModel.ListByProjectId(c.Db, proj.ID)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project budgets error")))
		return
	}

	budgetResp := GenerateProjectBudgetResp(budgets)

	ctx.JSON(http.StatusOK, api.Success(&DetailReply{
		Project: *NormalizeWalletAddrInProject(proj),
		Budgets: budgetResp,
	}))
}

func (c *ProjectsController) ShowBudgets(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	db := api.ForContextOnlyDB(ctx)

	budgets, err := model.ProjectBudgetModel.ListByProjectId(db, uint(id))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project budgets error")))
		return
	}

	budgetResp := GenerateProjectBudgetResp(budgets)
	ctx.JSON(http.StatusOK, api.Success(budgetResp))
}

func (c *ProjectsController) Create(ctx *gin.Context) {
	req := CreateReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		log.Error().Msgf("Parse request error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	httpCode, reply := c.ProjectsService.Create(ctx, &req)

	ctx.JSON(httpCode, reply)
}

func (c *ProjectsController) Update(ctx *gin.Context) {
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

	httpCode, reply := c.ProjectsService.Update(ctx, id, &req)

	ctx.JSON(httpCode, reply)
}

func (c *ProjectsController) Close(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	httpCode, reply := c.ProjectsService.Close(ctx, id)

	ctx.JSON(httpCode, reply)
}

func (c *ProjectsController) UpdateStaffs(ctx *gin.Context) {
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

	httpCode, reply := c.ProjectsService.UpdateStaffs(ctx, id, &req)

	ctx.JSON(httpCode, reply)
}

func (c *ProjectsController) UpdateBudget(ctx *gin.Context) {
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

	httpCode, reply := c.ProjectsService.UpdateBudget(ctx, id, &req)

	ctx.JSON(httpCode, reply)
}

func (c *ProjectsController) AddRelatedProposal(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}
	proposalIDs := ctx.QueryArray("proposalIDs")

	httpCode, reply := c.ProjectsService.AddRelatedProposal(ctx, id, proposalIDs)

	ctx.JSON(httpCode, reply)
}

func (c *ProjectsController) MyProjects(ctx *gin.Context) {
	user := api.ForContextOnlyUser(ctx)

	page := api.ParseAndConvertPageParam(ctx)

	projects, total, err := model.ProjectModel.ListBySponsorOrMember(c.Db, common.FormatUserWallet(user.Wallet), page)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list projects error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  page.Page,
		Size:  page.Size,
		Total: total,
		Rows: lo.Map(projects, func(project *model.Project, _ int) model.Project {
			return *NormalizeWalletAddrInProject(project)
		}),
	}))
}
