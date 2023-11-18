package project

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Project ------ ------

type (
	CreateReq struct {
		LogoStr string `json:"logo"` // base64 encoded image string
		Name    string `json:"name"`
		Intro   string `json:"intro"`
		Desc    string `json:"desc"`

		Sponsors  []string `json:"sponsors"`
		Members   []string `json:"members"`
		Proposals []string `json:"proposals"`

		Budgets []*BudgetParam `json:"budgets"`
	}
	BudgetParam struct {
		Name        string          `json:"name"`
		TotalAmount decimal.Decimal `json:"total_amount"`
	}
	UpdateReq struct {
		LogoStr string `json:"logo"`
		Name    string `json:"name"`
		Intro   string `json:"intro"`
		Desc    string `json:"desc"`
	}
	DetailReply struct {
		model.Project
		Budgets []*model.ProjectBudget `json:"budgets"`
	}
	UpdateBudgetReq struct {
		Id          uint            `json:"id"`
		AssetName   string          `json:"asset_name"`
		TotalAmount decimal.Decimal `json:"total_amount"`
	}
)

// Create `POST /projects`
//
//	@Summary		Create a project with passed in data
//	@Description	This api create a project record with passed in data
//	@Router			/projects [post]
//	@Tags			Project
//	@Param			request	body		CreateReq	true	"new project request data"
//
//	@Success		200		{object}	api.Reply
func Create(ctx *gin.Context) {
	req := CreateReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	// convert all wallet to lower case
	sponsors := lo.Map[string](req.Sponsors, func(item string, _ int) string {
		return strings.ToLower(item)
	})
	members := lo.Map[string](req.Members, func(item string, _ int) string {
		return strings.ToLower(item)
	})
	// remove duplicate sponsors and members
	sponsors = lo.Uniq[string](sponsors)
	members = lo.Uniq[string](members)
	// remove sponsors from members
	members = lo.Without[string](members, sponsors...)

	// remove duplicate proposals
	proposals := lo.Uniq[string](req.Proposals)

	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(user.Wallet, api.ObjProj, api.ActCreate)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	tx := db.Begin()
	// save project
	proj := model.Project{
		Name:      req.Name,
		Intro:     req.Intro,
		Desc:      req.Desc,
		Status:    model.ProjectStatusOpen,
		Sponsors:  sponsors,
		Members:   members,
		Proposals: proposals,
		Creator:   user.Wallet,
	}
	err = model.ProjectModel.CreateOrUpdate(tx, &proj)
	if err != nil {
		tx.Rollback()
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	// save project budgets
	budgets := lo.Map[*BudgetParam, *model.ProjectBudget](req.Budgets, func(item *BudgetParam, _ int) *model.ProjectBudget {
		return &model.ProjectBudget{
			ProjectID:    proj.ID,
			AssetName:    item.Name,
			TotalAmount:  item.TotalAmount,
			UsedAmount:   decimal.Zero,
			RemainAmount: item.TotalAmount,
		}
	})
	err = model.ProjectBudgetModel.Create(tx, budgets)
	if err != nil {
		tx.Rollback()
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	// Withdraw asset from treasure
	for _, budget := range budgets {
		err = model.TreasuryAssetHelper.WithdrawTreasureAsset(tx, budget.AssetName, budget.TotalAmount, user.Wallet, fmt.Sprintf("Create project %d by %s", proj.ID, user.Wallet))
		if err != nil {
			tx.Rollback()
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
	}

	// commit transaction
	tx.Commit()

	// Save logo image to S3
	logoUrl, err := sdk.GetAwsClient().UploadEntityLogo(proj.ID, "project", req.LogoStr)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	err = db.Model(&proj).Update("logo", logoUrl).Error
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	// add policies
	policies := [][]string{
		// p, proj_sponsor_1, proj_1, modify
		// p, proj_sponsor_1, proj_1, create_app
		// p, proj_sponsor_1, proj_1, u_member
		// p, proj_sponsor_1, proj_1, u_budget
		{fmt.Sprintf("%s%d", api.RoleProjSponsorPrefix, proj.ID), fmt.Sprintf("%s%d", api.ObjProjPrefix, proj.ID), api.ActModify},
		{fmt.Sprintf("%s%d", api.RoleProjSponsorPrefix, proj.ID), fmt.Sprintf("%s%d", api.ObjProjPrefix, proj.ID), api.ActCreateApplication},
		{fmt.Sprintf("%s%d", api.RoleProjSponsorPrefix, proj.ID), fmt.Sprintf("%s%d", api.ObjProjPrefix, proj.ID), api.ActUpdateMember},
		{fmt.Sprintf("%s%d", api.RoleProjSponsorPrefix, proj.ID), fmt.Sprintf("%s%d", api.ObjProjPrefix, proj.ID), api.ActUpdateBudget},
		//// p, proj_member_1, proj_1, modify
		//// p, proj_member_1, proj_1, create_app
		//{fmt.Sprintf("%s%d", api.RoleProjMemberPrefix, proj.ID), fmt.Sprintf("%s%d", api.ObjProjPrefix, proj.ID), api.ActModify},
		//{fmt.Sprintf("%s%d", api.RoleProjMemberPrefix, proj.ID), fmt.Sprintf("%s%d", api.ObjProjPrefix, proj.ID), api.ActCreateApplication},
	}
	_, err = enforcer.AddPolicies(policies)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	// add roles
	sponsorGroupingPolicies := lo.Map(req.Sponsors, func(sponsor string, _ int) []string {
		// g, 0xc13..1283 proj_sponsor_1
		return []string{strings.ToLower(sponsor), fmt.Sprintf("%s%d", api.RoleProjSponsorPrefix, proj.ID)}
	})
	//memberGroupingPolicies := lo.Map(req.Members, func(member string, _ int) []string {
	//	// g, 0xc13..1283 proj_member_1
	//	return []string{strings.ToLower(member), fmt.Sprintf("%s%d", api.RoleProjMemberPrefix, proj.ID)}
	//})
	//groupingPolicies := append(memberGroupingPolicies, sponsorGroupingPolicies...)
	_, err = enforcer.AddGroupingPolicies(sponsorGroupingPolicies)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	err = enforcer.SavePolicy()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	// send notification
	push := api.ForContextOnlyPush(ctx)
	staffs := append(sponsors, members...)
	go func(push []sdk.Pusher, staffs []string, projectID uint, projectName string) {
		title, body, data := sdk.GenerateProjectStaffAddNotificationParams(projectID, projectName)
		for _, p := range push {
			err := p.PushToWallets(staffs, title, body, data)
			if err != nil {
				log.Error().Msgf("push to %v failed: %s", staffs, err)
			}
		}
	}(push, staffs, proj.ID, proj.Name)

	ctx.JSON(http.StatusOK, api.Success(proj))
}

// Update `PUT /projects/:id`
//
//	@Summary		Update project information
//	@Description	This api update a project record with passed in data
//	@Router			/projects/:id [put]
//	@Tags			Project
//	@Param			id		path		string		true	"id of the project"
//	@Param			request	body		UpdateReq	true	"update project info"
//
//	@Success		200		{object}	api.Reply
func Update(ctx *gin.Context) {
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

	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(user.Wallet, fmt.Sprintf("%s%d", api.ObjProjPrefix, id), api.ActModify)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	proj, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if proj == nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id)))
		return
	}

	// project can be updated only when its status is 'open'
	if proj.Status != model.ProjectStatusOpen {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d can't be update", id)))
		return
	}

	logoUrl, err := sdk.GetAwsClient().UploadEntityLogo(proj.ID, "project", req.LogoStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("upload logo for project %d failed, err: %+v", id, err)))
		return
	}

	// update logo and name
	proj.Logo = logoUrl
	proj.Name = req.Name
	proj.Intro = req.Intro
	proj.Desc = req.Desc
	err = model.ProjectModel.CreateOrUpdate(db, proj)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// Close
// POST /project/:id/close
//
//	@Summary		Close a project
//	@Description	This api close specified project, admin permission is required for this operation
//	@Router			/projects/:id/close [post]
//	@Tags			Project
//	@Param			id	path		number	true	"project ID"
//
//	@Success		200	{object}	api.Reply
func Close(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(user.Wallet, api.ObjProj, api.ActClose)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	project, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if project == nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id)))
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
			CreateTs:   model.GetCurrentUtcEpochSecond(),
			UpdateTs:   model.GetCurrentUtcEpochSecond(),
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

// Detail `GET /project/:id`
//
//	@Summary	show detail of a project
//	@Tags		Project
//	@Param		id	path	int	true	"guild id"
//	@Router		/projects/:id [get]
//
//	@Success	200	{object}	api.Reply{data=DetailReply}
func Detail(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	db := api.ForContextOnlyDB(ctx)

	proj, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if proj == nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id)))
		return
	}

	budgets, err := model.ProjectBudgetModel.ListByProjectId(db, proj.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&DetailReply{
		Project: *proj,
		Budgets: budgets,
	}))
}

// List `GET /projects?status=open&page=1&size=10&sort_field=created_at&sort_order=desc`
//
//	@Summary		List all projects match the query params
//	@Description	This api parses passed in pagination query params,
//	@Router			/projects [get]
//	@Tags			Project
//	@Param			status		query		string	false	"status array, e.g. 'open,pending_close'"	Enum(open pending_close closed)
//	@Param			page		query		string	false	"which page"
//	@Param			size		query		string	false	"size of each page"
//	@Param			sort_field	query		string	false	"sort by which field"
//	@Param			sort_order	query		string	false	"order of sort"	Enum(asc desc)
//
//	@Success		200			{object}	api.Reply{data=api.ListReplyData{rows=model.Project}}
func List(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)

	status := ctx.Query("status")
	page := api.ParseAndConvertPageParam(ctx)

	showSpecialProjectsParam := ctx.Query("show_special")
	showSpecialProjectFlag := strings.EqualFold(showSpecialProjectsParam, "true")

	projects, total, err := model.ProjectModel.List(db, status, page, showSpecialProjectFlag)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  page.Page,
		Size:  page.Size,
		Total: total,
		Rows:  projects,
	}))
}

// MyProjects  list my projects
//
//	`GET /projects/my_projects?page=1&size=10&sort_field=created_at&sort_order=desc`
//
//	@Summary	list my projects
//	@Tags		Project
//	@Accept		json
//	@Produce	json
//	@Param		page		query		int		false	"page number, default: 1"
//	@Param		size		query		int		false	"page size, default: 10"
//	@Param		sort_field	query		string	false	"sort field, default: created_at"
//	@Param		sort_order	query		string	false	"sort order, default: desc"
//	@Success	200			{object}	api.Reply{data=api.ListReplyData{rows=model.Project}}
//	@Router		/projects/my_projects [get]
func MyProjects(ctx *gin.Context) {
	user, db := api.ForContextUserAndDB(ctx)

	page := api.ParseAndConvertPageParam(ctx)

	projects, total, err := model.ProjectModel.ListBySponsorOrMember(db, user.Wallet, page)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  page.Page,
		Size:  page.Size,
		Total: total,
		Rows:  projects,
	}))
}

type UpdateStaffsReq struct {
	Action   string   `json:"action"` // `add` or `remove`
	Sponsors []string `json:"sponsors"`
	Members  []string `json:"members"`
}

// UpdateStaffs update project sponsors and members
//
//	@Summary	update project sponsors and members
//	@Tags		Project
//	@Accept		json
//	@Produce	json
//	@Param		id			path		int				true	"project id"
//	@Param		JsonBody	body		UpdateStaffsReq	true	"request json body"
//	@Success	200			{object}	api.Reply
//	@Router		/projects/{id}/update_staffs [post]
func UpdateStaffs(ctx *gin.Context) {
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

	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	if req.Sponsors != nil && len(req.Sponsors) != 0 {
		ok, err := enforcer.Enforce(user.Wallet, fmt.Sprintf("%s%d", api.ObjProjPrefix, id), api.ActUpdateSponsor)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
		if !ok {
			ctx.JSON(http.StatusForbidden, api.Forbidden())
			return
		}
	}
	if req.Members != nil && len(req.Members) != 0 {
		ok, err := enforcer.Enforce(user.Wallet, fmt.Sprintf("%s%d", api.ObjProjPrefix, id), api.ActUpdateMember)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
		if !ok {
			ctx.JSON(http.StatusForbidden, api.Forbidden())
			return
		}
	}

	// convert all wallet to lower case
	sponsors := lo.Map[string](req.Sponsors, func(item string, _ int) string {
		return strings.ToLower(item)
	})
	members := lo.Map[string](req.Members, func(item string, _ int) string {
		return strings.ToLower(item)
	})

	proj, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if proj == nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id)))
		return
	}

	// project can be updated only when its status is 'open'
	if proj.Status != model.ProjectStatusOpen {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("project can't be update")))
		return
	}

	// ------ ------ ------ ------ ------ ------ ------ ------ ------

	if req.Action == "add" {
		tx := db.Begin()

		if req.Sponsors != nil && len(req.Sponsors) != 0 {
			// add project sponsors
			proj.Sponsors = append(proj.Sponsors, sponsors...)
			// remove duplicate sponsors
			proj.Sponsors = lo.Uniq[string](proj.Sponsors)
			// remove sponsors from members
			proj.Sponsors = lo.Without[string](proj.Sponsors, proj.Members...)
			err = model.ProjectModel.CreateOrUpdate(tx, proj)
			if err != nil {
				tx.Rollback()

				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
				return
			}

			// add roles for new sponsors
			newSponsorGroupingPolicies := lo.Map(req.Sponsors, func(sponsor string, _ int) []string {
				// g, 0xc13..1283 proj_sponsor_1
				return []string{strings.ToLower(sponsor), fmt.Sprintf("%s%d", api.RoleProjSponsorPrefix, proj.ID)}
			})
			_, err = enforcer.AddGroupingPolicies(newSponsorGroupingPolicies)
			if err != nil {
				tx.Rollback()

				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
				return
			}
			err = enforcer.SavePolicy()
			if err != nil {
				tx.Rollback()

				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
				return
			}
		}

		if req.Members != nil && len(req.Members) != 0 {
			// add project members
			proj.Members = append(proj.Members, members...)
			// remove duplicate members
			proj.Members = lo.Uniq[string](proj.Members)
			// remove members from sponsors
			proj.Members = lo.Without[string](proj.Members, proj.Sponsors...)
			err = model.ProjectModel.CreateOrUpdate(tx, proj)
			if err != nil {
				tx.Rollback()

				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
				return
			}

			//// add roles for new members
			//newMemberGroupingPolicies := lo.Map(req.Members, func(member string, _ int) []string {
			//	// g, 0xc13..1283 proj_member_1
			//	return []string{strings.ToLower(member), fmt.Sprintf("%s%d", api.RoleProjMemberPrefix, proj.ID)}
			//})
			//_, err = enforcer.AddGroupingPolicies(newMemberGroupingPolicies)
			//if err != nil {
			//	ctx.JSON(http.StatusInternalServerError, err)
			//	return
			//}
		}

		tx.Commit()

		// send notification
		push := api.ForContextOnlyPush(ctx)
		staffs := append(sponsors, members...)
		go func(push []sdk.Pusher, staffs []string, projectID uint, projectName string) {
			title, body, data := sdk.GenerateProjectStaffAddNotificationParams(projectID, projectName)
			for _, p := range push {
				err := p.PushToWallets(staffs, title, body, data)
				if err != nil {
					log.Error().Msgf("push to %+v failed: %s", staffs, err)
				}
			}
		}(push, staffs, proj.ID, proj.Name)
	} else if req.Action == "remove" {
		tx := db.Begin()

		if req.Sponsors != nil && len(req.Sponsors) != 0 {
			// remove project sponsors
			proj.Sponsors = lo.Without[string](proj.Sponsors, sponsors...)
			err = model.ProjectModel.CreateOrUpdate(tx, proj)
			if err != nil {
				tx.Rollback()

				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
				return
			}

			// remove roles for old sponsors
			oldSponsorGroupingPolicies := lo.Map(proj.Sponsors, func(sponsor string, _ int) []string {
				// g, 0xc13..1283 proj_sponsor_1
				return []string{sponsor, fmt.Sprintf("%s%d", api.RoleProjSponsorPrefix, proj.ID)}
			})
			_, err = enforcer.RemoveGroupingPolicies(oldSponsorGroupingPolicies)
			if err != nil {
				tx.Rollback()

				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
				return
			}
			err = enforcer.SavePolicy()
			if err != nil {
				tx.Rollback()

				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
				return
			}
		}

		if req.Members != nil && len(req.Members) != 0 {
			// remove project members
			proj.Members = lo.Without[string](proj.Members, members...)
			err = model.ProjectModel.CreateOrUpdate(tx, proj)
			if err != nil {
				tx.Rollback()

				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
				return
			}

			//// remove roles for old members
			//oldMemberGroupingPolicies := lo.Map(proj.Members, func(member string, _ int) []string {
			//	// g, 0xc13..1283 proj_member_1
			//	return []string{member, fmt.Sprintf("%s%d", api.RoleProjMemberPrefix, proj.ID)}
			//})
			//_, err = enforcer.RemoveGroupingPolicies(oldMemberGroupingPolicies)
			//if err != nil {
			//	ctx.JSON(http.StatusInternalServerError, err)
			//	return
			//}
		}

		tx.Commit()

		// send notification
		push := api.ForContextOnlyPush(ctx)
		staffs := append(sponsors, members...)
		go func(push []sdk.Pusher, staffs []string, projectID uint, projectName string) {
			title, body, data := sdk.GenerateProjectStaffRemoveNotificationParams(projectID, projectName)
			for _, p := range push {
				err := p.PushToWallets(staffs, title, body, data)
				if err != nil {
					log.Error().Msgf("push to %+v failed: %s", staffs, err)
				}
			}
		}(push, staffs, proj.ID, proj.Name)
	}

	// ------ ------ ------ ------ ------ ------ ------ ------ ------

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Project Budget ------ ------

// UpdateBudget update project budget
//
//	@Summary	Update project budget
//	@Tags		Project
//	@Accept		json
//	@Produce	json
//	@Param		id			path		int				true	"project id"
//	@Param		JsonBody	body		UpdateBudgetReq	true	"request json body"
//	@Success	200			{object}	api.Reply
//	@Router		/projects/{id}/update_budget [post]
func UpdateBudget(ctx *gin.Context) {
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

	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(user.Wallet, fmt.Sprintf("%s%d", api.ObjProjPrefix, id), api.ActUpdateBudget)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	proj, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if proj == nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id)))
		return
	}

	// project can be updated only when its status is 'open'
	if proj.Status != model.ProjectStatusOpen {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d can't be update", id)))
		return
	}

	budget, err := model.ProjectBudgetModel.Detail(db, req.Id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	// update `TotalAmount`
	budget.TotalAmount = req.TotalAmount
	budget.RemainAmount = budget.TotalAmount.Sub(budget.UsedAmount)
	err = model.ProjectBudgetModel.Update(db, budget)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Project Proposals ------ ------

// AddRelatedProposal `POST /projects/:id/add_related_proposal?proposalIDs=1&proposalIDs=2`
//
//	@Summary	Add related proposals to the project
//	@Router		/projects/:id/add_related_proposal [post]
//	@Tags		Project
//	@Param		id			path		number		true	"project ID"
//	@Param		proposalIDs	query		[]string	true	"proposal ID list"
//
//	@Success	200			{object}	api.Reply
func AddRelatedProposal(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}
	proposalIDs := ctx.QueryArray("proposalIDs")

	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(user.Wallet, fmt.Sprintf("%s%d", api.ObjProjPrefix, id), api.ActModify)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	proj, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if proj == nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id)))
		return
	}

	// project can be updated only when its status is 'open'
	if proj.Status != model.ProjectStatusOpen {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d can't be update", id)))
		return
	}

	proj.Proposals = append(proj.Proposals, proposalIDs...)
	// remove duplicate proposals
	proj.Proposals = lo.Uniq[string](proj.Proposals)
	err = model.ProjectModel.CreateOrUpdate(db, proj)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}
