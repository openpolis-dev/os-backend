package project

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
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

	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(user.Wallet, api.ObjProj, api.ActCreate)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, err)
		return
	}

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
	err = model.ProjectModel.CreateOrUpdate(tx, &proj)
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

	// add policies
	policies := [][]string{
		// p, proj_sponsor_1, proj_1, modify
		// p, proj_member_1, proj_1, modify
		{fmt.Sprintf("%s%d", api.RoleProjSponsorPre, proj.ID), fmt.Sprintf("%s%d", api.ObjProjPre, proj.ID), api.ActModify},
		{fmt.Sprintf("%s%d", api.RoleProjMemberPre, proj.ID), fmt.Sprintf("%s%d", api.ObjProjPre, proj.ID), api.ActModify},
	}
	_, err = enforcer.AddPolicies(policies)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}
	// add roles
	sponsorGroupingPolicies := lo.Map(req.Sponsors, func(sponsor string, _ int) []string {
		// g, 0xc13..1283 proj_sponsor_1
		return []string{strings.ToLower(sponsor), fmt.Sprintf("%s%d", api.RoleProjSponsorPre, proj.ID)}
	})
	memberGroupingPolicies := lo.Map(req.Members, func(member string, _ int) []string {
		// g, 0xc13..1283 proj_member_1
		return []string{strings.ToLower(member), fmt.Sprintf("%s%d", api.RoleProjMemberPre, proj.ID)}
	})
	groupingPolicies := append(memberGroupingPolicies, sponsorGroupingPolicies...)
	_, err = enforcer.AddGroupingPolicies(groupingPolicies)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}
	err = enforcer.SavePolicy()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

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

	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(user.Wallet, fmt.Sprintf("%s%d", api.ObjProjPre, id), api.ActModify)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, err)
		return
	}

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

func Close(ctx *gin.Context) {
	// TODO
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

	budgets, err := model.ProjectBudgetModel.List(db, proj.ID)
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
	user, db := api.ForContextUserAndDB(ctx)

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

	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(user.Wallet, fmt.Sprintf("%s%d", api.ObjProjPre, id), api.ActModify)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, err)
		return
	}

	proj, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	// remove roles for ole sponsors
	oldSponsorGroupingPolicies := lo.Map(proj.Sponsors, func(sponsor string, _ int) []string {
		// g, 0xc13..1283 proj_sponsor_1
		return []string{sponsor, fmt.Sprintf("%s%d", api.RoleProjSponsorPre, proj.ID)}
	})
	_, err = enforcer.RemoveGroupingPolicies(oldSponsorGroupingPolicies)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	// update project sponsors
	proj.Sponsors = req.Sponsors
	err = model.ProjectModel.CreateOrUpdate(db, proj)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	// add roles for new sponsors
	newSponsorGroupingPolicies := lo.Map(req.Sponsors, func(sponsor string, _ int) []string {
		// g, 0xc13..1283 proj_sponsor_1
		return []string{strings.ToLower(sponsor), fmt.Sprintf("%s%d", api.RoleProjSponsorPre, proj.ID)}
	})
	_, err = enforcer.AddGroupingPolicies(newSponsorGroupingPolicies)
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

	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(user.Wallet, fmt.Sprintf("%s%d", api.ObjProjPre, id), api.ActModify)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, err)
		return
	}

	proj, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	// remove roles for ole members
	oldMemberGroupingPolicies := lo.Map(proj.Members, func(member string, _ int) []string {
		// g, 0xc13..1283 proj_member_1
		return []string{member, fmt.Sprintf("%s%d", api.RoleProjMemberPre, proj.ID)}
	})
	_, err = enforcer.RemoveGroupingPolicies(oldMemberGroupingPolicies)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	// update project members
	proj.Members = req.Members
	err = model.ProjectModel.CreateOrUpdate(db, proj)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	// add roles for new members
	newMemberGroupingPolicies := lo.Map(req.Members, func(member string, _ int) []string {
		// g, 0xc13..1283 proj_member_1
		return []string{strings.ToLower(member), fmt.Sprintf("%s%d", api.RoleProjMemberPre, proj.ID)}
	})
	_, err = enforcer.AddGroupingPolicies(newMemberGroupingPolicies)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Project Budget ------ ------

type UpdateBudgetReq struct {
	ID          uint   `json:"id"`
	TotalAmount uint64 `json:"totalAmount"`
}

// UpdateBudget `POST /projects/:id/update_budget`
func UpdateBudget(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, _ := strconv.Atoi(idParam)

	req := UpdateBudgetReq{}
	_ = ctx.BindJSON(&req)

	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(user.Wallet, fmt.Sprintf("%s%d", api.ObjProjPre, id), api.ActModify)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, err)
		return
	}

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

	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(user.Wallet, fmt.Sprintf("%s%d", api.ObjProjPre, id), api.ActModify)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, err)
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, err)
		return
	}

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
