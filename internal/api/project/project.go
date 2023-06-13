package project

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Project ------ ------

type (
	CreateReq struct {
		Logo string `json:"logo"`
		Name string `json:"name"`

		Sponsors  []string `json:"sponsors"`
		Members   []string `json:"members"`
		Proposals []string `json:"proposals"`

		Budgets []*BudgetParam `json:"budgets"`
	}
	BudgetParam struct {
		Name        string `json:"name"`
		TotalAmount uint64 `json:"totalAmount"`
	}
)

// Create `POST /projects`
func Create(ctx *gin.Context) {
	req := CreateReq{}
	_ = ctx.BindJSON(&req)

	db := api.ForContextOnlyDB(ctx)

	tx := db.Begin()
	// save project
	proj := model.Project{
		Logo:      req.Logo,
		Name:      req.Name,
		Status:    api.ProjectStatusOpen,
		Sponsors:  req.Sponsors,
		Members:   req.Members,
		Proposals: req.Proposals,
	}
	err := model.ProjectModel.CreateOrUpdate(tx, &proj)
	if err != nil {
		tx.Rollback()
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}
	// save project budgets
	budgets := lo.Map[*BudgetParam, *model.ProjectBudget](req.Budgets, func(item *BudgetParam, _ int) *model.ProjectBudget {
		return &model.ProjectBudget{
			ProjectID:   proj.ID,
			Name:        item.Name,
			TotalAmount: item.TotalAmount,
		}
	})
	err = model.ProjectBudgetModel.Create(tx, budgets)
	if err != nil {
		tx.Rollback()
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}
	// commit transaction
	tx.Commit()

	ctx.JSON(http.StatusOK, api.Success(nil))
}

type UpdateReq struct {
	Logo string `json:"logo"`
	Name string `json:"name"`
}

// Update `PUT /projects/:id`
func Update(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, _ := strconv.Atoi(idParam)

	req := UpdateReq{}
	_ = ctx.BindJSON(&req)

	db := api.ForContextOnlyDB(ctx)

	proj, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}
	// update logo and name
	proj.Logo = req.Logo
	proj.Name = req.Name
	err = model.ProjectModel.CreateOrUpdate(db, proj)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// Close
// POST /project/:id/close
func Close(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, _ := strconv.Atoi(idParam)

	user, db, _ := api.ForContext(ctx)

	project, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	if project.Status != model.ProjectStatusOpen {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("project %d status is not suit for closing", id),
		})
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		application := model.Application{
			Type:       model.ApplicationCloseProject,
			Applicant:  user.Wallet,
			State:      model.ApplicationStateOpen,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			EntityType: "project",
			EntityId:   project.ID,
		}
		err = model.NewApplicationRecord(tx, &application)
		if err != nil {
			return err
		}
		project.Status = model.ProjectStatusPendingClose
		return tx.Save(project).Error
	})

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("update project status error: %s", err.Error()),
		})
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

type DetailReply struct {
	model.Project
	Budgets []*model.ProjectBudget `json:"budgets"`
}

// Detail `GET /project/:id`
func Detail(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, _ := strconv.Atoi(idParam)

	db := api.ForContextOnlyDB(ctx)

	proj, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	budgets, err := model.ProjectBudgetModel.ListByProjectId(db, proj.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&DetailReply{
		Project: *proj,
		Budgets: budgets,
	}))
}

// List `GET /projects?status=open&page=1&size=10&sort_field=created_at&sort_order=desc`
func List(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)

	status := ctx.Query("status")
	page := api.ParseAndConvertPageParam(ctx)

	projects, err := model.ProjectModel.List(db, status, page)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, api.Success(projects))
}

// MyProjects `GET /projects/my?page=1&size=10&sort_field=created_at&sort_order=desc`
func MyProjects(ctx *gin.Context) {
	user, db, _ := api.ForContext(ctx)

	page := api.ParseAndConvertPageParam(ctx)

	projects, err := model.ProjectModel.ListBySponsorOrMember(db, user.Wallet, page)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, api.Success(projects))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Project Sponsors/Members ------ ------

type UpdateSponsorsReq struct {
	Sponsors []string `json:"sponsors"`
}

// UpdateSponsors `POST /projects/:id/update_sponsors`
func UpdateSponsors(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, _ := strconv.Atoi(idParam)

	req := UpdateSponsorsReq{}
	_ = ctx.BindJSON(&req)

	db := api.ForContextOnlyDB(ctx)

	proj, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	proj.Sponsors = req.Sponsors
	err = model.ProjectModel.CreateOrUpdate(db, proj)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

type UpdateMembersReq struct {
	Members []string `json:"members"`
}

// UpdateMembers `POST /projects/:id/update_members`
func UpdateMembers(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, _ := strconv.Atoi(idParam)

	req := UpdateMembersReq{}
	_ = ctx.BindJSON(&req)

	db := api.ForContextOnlyDB(ctx)

	proj, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	proj.Members = req.Members
	err = model.ProjectModel.CreateOrUpdate(db, proj)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Project Budget ------ ------

type UpdateBudgetReq struct {
	ID          uint   `json:"ID"`
	TotalAmount uint64 `json:"totalAmount"`
}

// UpdateBudget `POST /projects/:id/update_budget`
func UpdateBudget(ctx *gin.Context) {
	//idParam := ctx.Param("id")
	//id, _ := strconv.Atoi(idParam)

	req := UpdateBudgetReq{}
	_ = ctx.BindJSON(&req)

	db := api.ForContextOnlyDB(ctx)

	budget, err := model.ProjectBudgetModel.Detail(db, req.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}
	// update `TotalAmount`
	budget.TotalAmount = req.TotalAmount
	err = model.ProjectBudgetModel.Update(db, budget)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Project Proposals ------ ------

type AddProposalReq struct {
	ProposalID []string `json:"ids"`
}

// AddRelatedProposal `POST /projects/:id/add_related_proposal/:proposal_id`
func AddRelatedProposal(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, _ := strconv.Atoi(idParam)
	proposalID := ctx.Param("proposal_id")

	db := api.ForContextOnlyDB(ctx)

	proj, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	proj.Proposals = append(proj.Proposals, proposalID)
	err = model.ProjectModel.CreateOrUpdate(db, proj)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}
