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
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
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

		ScrBudget  decimal.Decimal `json:"scr_budget"`
		UsdcBudget decimal.Decimal `json:"usdc_budget"`

		SIP          string `json:"SIP"`
		Category     string `json:"Category"`
		ApprovalLink string `json:"ApprovalLink"`
		OverLink     string `json:"OverLink"`
		Deliverable  string `json:"Deliverable"`
		PlanTime     string `json:"PlanTime"`
		ContantWay   string `json:"ContantWay"`
		OfficialLink string `json:"OfficialLink"`
	}

	ProjectBudgetResp struct {
		AssetName    string `json:"asset_name"`
		TotalAmount  string `json:"total_amount"`
		UsedAmount   string `json:"used_amount"`
		RemainAmount string `json:"remain_amount"`

		AdvanceRatio        string `json:"advance_ratio"`
		TotalAdvanceAmount  string `json:"total_advance_amount"`
		UsedAdvanceAmount   string `json:"used_advance_amount"`
		RemainAdvanceAmount string `json:"remain_advance_amount"`
	}

	UpdateReq struct {
		LogoStr string `json:"logo"`
		// Name    string `json:"name"`
		// Intro   string `json:"intro"`
		Desc string `json:"desc"`

		Sponsors     []string `json:"sponsors"`
		OverLink     string   `json:"OverLink"`
		ContantWay   string   `json:"ContantWay"`
		OfficialLink string   `json:"OfficialLink"`
	}
	DetailReply struct {
		model.Project
		Budgets []*ProjectBudgetResp `json:"budgets"`
	}
	UpdateBudgetReq struct {
		Id          uint            `json:"id"`
		AssetName   string          `json:"asset_name"`
		TotalAmount decimal.Decimal `json:"total_amount"`
	}
)

// Create `POST /projects`
//
//	@summary		Create a project with passed in data
//	@description	This api create a project record with passed in data
//	@router			/projects [post]
//	@tags			Project
//	@param			request	body		CreateReq	true	"new project request data"
//	@success		200		{object}	api.Reply
func Create(ctx *gin.Context) {
	req := CreateReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		log.Error().Msgf("Parse request error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	// convert all wallet to checksum address
	sponsors := lo.Map[string](req.Sponsors, func(item string, _ int) string {
		return common.FormatUserWallet(item)
	})
	members := lo.Map[string](req.Members, func(item string, _ int) string {
		return common.FormatUserWallet(item)
	})
	// remove duplicate sponsors and members
	sponsors = lo.Uniq[string](sponsors)
	members = lo.Uniq[string](members)
	// remove sponsors from members
	members = lo.Without[string](members, sponsors...)

	// remove duplicate proposals
	proposals := lo.Uniq[string](req.Proposals)

	user, enforcer, db, _ := api.ForContext(ctx)

	// Check permission, Only CityHall member can create project
	ok, err := enforcer.HasRoleForUser(common.FormatUserWallet(user.Wallet), internal.RoleHall)
	if err != nil {
		log.Error().Msgf("check permission error %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error")))
		return
	}

	if !ok {
		log.Warn().Msgf("permission deny for user %s", common.FormatUserWallet(user.Wallet))
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
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
		Creator:   common.FormatUserWallet(user.Wallet),
		CreatedAt: time.Now().In(internal.ProjectTimezone),
		UpdatedAt: time.Now().In(internal.ProjectTimezone),
		CreateTs:  model.GetCurrentUtcEpochSecond(),
		UpdateTs:  model.GetCurrentUtcEpochSecond(),

		SIP:          req.SIP,
		Category:     req.Category,
		ApprovalLink: req.ApprovalLink,
		OverLink:     req.OverLink,
		Deliverable:  req.Deliverable,
		PlanTime:     req.PlanTime,
		ContantWay:   req.ContantWay,
		OfficialLink: req.OfficialLink,
	}
	err = model.ProjectModel.CreateOrUpdate(tx, &proj)
	if err != nil {
		tx.Rollback()
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create project error")))
		return
	}

	var budgets []*model.ProjectBudget
	if req.ScrBudget != decimal.Zero {
		budgets = append(budgets, &model.ProjectBudget{
			ProjectID:    proj.ID,
			AssetName:    "SCR",
			TotalAmount:  req.ScrBudget,
			RemainAmount: req.ScrBudget,
			CreateTs:     model.GetCurrentUtcEpochSecond(),
			UpdateTs:     model.GetCurrentUtcEpochSecond(),
		})
	}

	if req.UsdcBudget != decimal.Zero {
		budgets = append(budgets, &model.ProjectBudget{
			ProjectID:    proj.ID,
			AssetName:    "USDC",
			TotalAmount:  req.UsdcBudget,
			RemainAmount: req.UsdcBudget,
			CreateTs:     model.GetCurrentUtcEpochSecond(),
			UpdateTs:     model.GetCurrentUtcEpochSecond(),
		})
	}

	if len(budgets) > 0 {
		err = model.ProjectBudgetModel.Create(tx, budgets)
	}

	// commit transaction
	tx.Commit()

	// Save logo image to S3
	logoUrl, err := sdk.GetAwsClient().UploadEntityLogo(proj.ID, "project", req.LogoStr)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("upload logo error")))
		return
	}
	err = db.Model(&proj).Update("logo", logoUrl).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("upload logo error")))
		return
	}

	// set policies for sponsor user
	err = model.SetWalletPermissionAsProjectSponsor(enforcer, proj.ID, []string{common.FormatUserWallet(user.Wallet)})
	if err != nil {
		log.Error().Msgf("set wallet permission error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("save policy error")))
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
//	@summary		Update project information
//	@description	This api update a project record with passed in data
//	@router			/projects/:id [put]
//	@tags			Project
//	@param			id		path		string		true	"id of the project"
//	@param			request	body		UpdateReq	true	"update project info"
//	@success		200		{object}	api.Reply
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
	proj, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		log.Error().Msgf("get project error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project error")))
		return
	}

	if proj == nil {
		log.Error().Msgf("project %d not exist", id)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id)))
		return
	}

	// check permission, updating some fields require city hall permission
	ok, err := enforcer.HasRoleForUser(common.FormatUserWallet(user.Wallet), internal.RoleHall)
	if err != nil {
		log.Error().Msgf("check permission error %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error")))
		return
	}

	requesterHasCityHallPerm := ok

	sponsorsBeforeUpdate := lo.SliceToMap(proj.Sponsors, func(item string) (string, bool) { return item, true })

	overProject := false
	if (len(req.OverLink) > 0) || (len(req.Sponsors) > 0) {
		// Only city hall can update overlink and sponsors
		if !requesterHasCityHallPerm {
			log.Warn().Msgf("permission deny for user %s", common.FormatUserWallet(user.Wallet))
			sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
			ctx.JSON(http.StatusForbidden, api.Forbidden())
			return
		}

		sponsors := lo.Uniq(lo.Map[string](req.Sponsors, func(item string, _ int) string {
			return common.FormatUserWallet(item)
		}))

		proj.Sponsors = sponsors
		proj.OverLink = req.OverLink
		if len(req.OverLink) > 0 {
			overProject = true
		}

		// Update sponsor permission
		// 1. Grant casbin permission for all existing sponsors
		// 2. Remove casbin permission for all removed sponsors
		removedSponsors := lo.Keys(lo.OmitByKeys(sponsorsBeforeUpdate, sponsors))

		log.Debug().Msgf("set sponsors permission: %+v, removed sponsors: %+v", sponsors, removedSponsors)

		err = model.SetWalletPermissionAsProjectSponsor(enforcer, proj.ID, sponsors)
		if err != nil {
			log.Error().Msgf("set sponsor permission error %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("set sponsor permission error")))
			return
		} else {
			log.Debug().Msgf("set project sponsor permission for %s", sponsors)
		}

		err = model.RemoveWalletPermissionFromProjectSponsor(enforcer, proj.ID, removedSponsors)
		if err != nil {
			log.Error().Msgf("unset sponsor permission error %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("unset sponsor permission error")))
			return
		} else {
			log.Debug().Msgf("unset project sponsor permission for %s", removedSponsors)
		}
	} else {
		if !requesterHasCityHallPerm {
			// only sponsor can update project data
			if len(proj.Sponsors) > 0 {
				if common.FormatUserWallet(proj.Sponsors[0]) != common.FormatUserWallet(user.Wallet) {
					log.Warn().Msgf("permission deny for user %s", common.FormatUserWallet(user.Wallet))
					sdk.LogForbiddenError(ctx, user.Wallet, "RoleSponsors", "access")
					ctx.JSON(http.StatusForbidden, api.Forbidden())
					return
				}
			} else {
				log.Warn().Msgf("permission deny for user %s", common.FormatUserWallet(user.Wallet))
				sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
				ctx.JSON(http.StatusForbidden, api.Forbidden())
				return
			}
		}
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
	// proj.Name = req.Name
	// proj.Intro = req.Intro
	proj.Desc = req.Desc
	proj.ContantWay = req.ContantWay
	proj.OfficialLink = req.OfficialLink

	if overProject {
		proj.Status = model.ProjectStatusClosed
	}

	proj.UpdateTs = model.GetCurrentUtcEpochSecond()
	proj.UpdatedAt = time.Now().In(internal.ProjectTimezone)

	err = model.ProjectModel.CreateOrUpdate(db, proj)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// Close
// POST /project/:id/close
//
//	@summary		Close a project
//	@description	This api close specified project, admin permission is required for this operation
//	@router			/projects/:id/close [post]
//	@tags			Project
//	@param			id	path		number	true	"project ID"
//	@success		200	{object}	api.Reply
func Close(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), internal.ObjProj, internal.ActClose)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.ObjProj, internal.ActClose)
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	project, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project error")))
		return
	}
	if project == nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id)))
		return
	}

	if project.Status != model.ProjectStatusOpen {
		err := fmt.Errorf("project %d current status %s is not suit for closing", id, project.Status)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		application := model.Application{
			Type:       model.ApplicationCloseProject,
			Applicant:  common.FormatUserWallet(user.Wallet),
			State:      model.ApplicationStateOpen,
			CreatedAt:  time.Now().In(internal.ProjectTimezone),
			UpdatedAt:  time.Now().In(internal.ProjectTimezone),
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
		project.UpdatedAt = time.Now().In(internal.ProjectTimezone)
		project.UpdateTs = model.GetCurrentUtcEpochSecond()
		return tx.Save(project).Error
	})

	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("close project failed")))
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// Detail `GET /project/:id`
//
//	@summary	show detail of a project
//	@tags		Project
//	@param		id	path	int	true	"guild id"
//	@router		/projects/:id [get]
//	@success	200	{object}	api.Reply{data=DetailReply}
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
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project error")))
		return
	}
	if proj == nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id)))
		return
	}

	budgets, err := model.ProjectBudgetModel.ListByProjectId(db, proj.ID)
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

// List `GET /projects?status=open&page=1&size=10&sort_field=created_at&sort_order=desc`
//
//	@summary		List all projects match the query params
//	@description	This api parses passed in pagination query params,
//	@router			/projects [get]
//	@tags			Project
//	@param			status		query		string	false	"status array, e.g. 'open,pending_close'"	Enum(open pending_close closed)
//	@param			keywords	query		string	false	"search keywords"
//	@param			wallet		query		string	false	"search wallet"
//	@param			page		query		string	false	"which page"
//	@param			size		query		string	false	"size of each page"
//	@param			sort_field	query		string	false	"sort by which field"
//	@param			sort_order	query		string	false	"order of sort"	Enum(asc desc)
//	@success		200			{object}	api.Reply{data=api.ListReplyData{rows=model.Project}}
func List(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)

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

	projects, total, err := model.ProjectModel.ListWithSearch(db, status, k, w, page, showSpecialProjectFlag)
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

// MyProjects  list my projects
//
//	`GET /projects/my_projects?page=1&size=10&sort_field=created_at&sort_order=desc`
//
//	@summary	list my projects
//	@tags		Project
//	@accept		json
//	@produce	json
//	@param		page		query		int		false	"page number, default: 1"
//	@param		size		query		int		false	"page size, default: 10"
//	@param		sort_field	query		string	false	"sort field, default: created_at"
//	@param		sort_order	query		string	false	"sort order, default: desc"
//	@success	200			{object}	api.Reply{data=api.ListReplyData{rows=model.Project}}
//	@router		/projects/my_projects [get]
func MyProjects(ctx *gin.Context) {
	user, db := api.ForContextUserAndDB(ctx)

	page := api.ParseAndConvertPageParam(ctx)

	projects, total, err := model.ProjectModel.ListBySponsorOrMember(db, common.FormatUserWallet(user.Wallet), page)
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

type UpdateStaffsReq struct {
	Action   string   `json:"action"` // `add` or `remove`
	Sponsors []string `json:"sponsors"`
	Members  []string `json:"members"`
}

// UpdateStaffs update project sponsors and members
//
//	@summary	update project sponsors and members
//	@tags		Project
//	@accept		json
//	@produce	json
//	@param		id			path		int				true	"project id"
//	@param		JsonBody	body		UpdateStaffsReq	true	"request json body"
//	@success	200			{object}	api.Reply
//	@router		/projects/{id}/update_staffs [post]
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
		permObject := buildProjectPermObject(id)
		ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), permObject, internal.ActUpdateSponsor)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
			return
		}
		if !ok {
			sdk.LogForbiddenError(ctx, user.Wallet, permObject, internal.ActUpdateSponsor)
			ctx.JSON(http.StatusForbidden, api.Forbidden())
			return
		}
	}
	if req.Members != nil && len(req.Members) != 0 {
		permObject := buildProjectPermObject(id)
		ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), permObject, internal.ActUpdateMember)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
			return
		}
		if !ok {
			sdk.LogForbiddenError(ctx, user.Wallet, permObject, internal.ActUpdateMember)
			ctx.JSON(http.StatusForbidden, api.Forbidden())
			return
		}
	}

	// convert all wallet to checksum address
	sponsors := lo.Map[string](req.Sponsors, func(item string, _ int) string {
		return common.FormatUserWallet(item)
	})
	members := lo.Map[string](req.Members, func(item string, _ int) string {
		return common.FormatUserWallet(item)
	})

	proj, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project error")))
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
			proj.UpdateTs = model.GetCurrentUtcEpochSecond()
			proj.UpdatedAt = time.Now().In(internal.ProjectTimezone)
			err = model.ProjectModel.CreateOrUpdate(tx, proj)
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
				return
			}

			// add roles for new sponsors
			newSponsorGroupingPolicies := lo.Map(req.Sponsors, func(sponsor string, _ int) []string {
				// g, 0xc13..1283 proj_sponsor_1
				return []string{common.FormatUserWallet(sponsor), fmt.Sprintf("%s%d", internal.RoleProjSponsorPrefix, proj.ID)}
			})
			_, err = enforcer.AddGroupingPolicies(newSponsorGroupingPolicies)
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
				return
			}
			err = enforcer.SavePolicy()
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
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
			proj.UpdateTs = model.GetCurrentUtcEpochSecond()
			proj.UpdatedAt = time.Now().In(internal.ProjectTimezone)
			err = model.ProjectModel.CreateOrUpdate(tx, proj)
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
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

				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
				return
			}

			// remove roles for old sponsors
			oldSponsorGroupingPolicies := lo.Map(proj.Sponsors, func(sponsor string, _ int) []string {
				// g, 0xc13..1283 proj_sponsor_1
				return []string{sponsor, fmt.Sprintf("%s%d", internal.RoleProjSponsorPrefix, proj.ID)}
			})
			_, err = enforcer.RemoveGroupingPolicies(oldSponsorGroupingPolicies)
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
				return
			}
			err = enforcer.SavePolicy()
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
				return
			}
		}

		if req.Members != nil && len(req.Members) != 0 {
			// remove project members
			proj.Members = lo.Without[string](proj.Members, members...)
			proj.UpdateTs = model.GetCurrentUtcEpochSecond()
			proj.UpdatedAt = time.Now().In(internal.ProjectTimezone)
			err = model.ProjectModel.CreateOrUpdate(tx, proj)
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
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
//	@summary	Update project budget
//	@tags		Project
//	@accept		json
//	@produce	json
//	@param		id			path		int				true	"project id"
//	@param		JsonBody	body		UpdateBudgetReq	true	"request json body"
//	@success	200			{object}	api.Reply
//	@router		/projects/{id}/update_budget [post]
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
	permObject := buildProjectPermObject(id)
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), permObject, internal.ActUpdateBudget)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, permObject, internal.ActUpdateBudget)
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	proj, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project error")))
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
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project budget error")))
		return
	}

	// update `TotalAmount`
	budget.TotalAmount = req.TotalAmount
	budget.RemainAmount = budget.TotalAmount.Sub(budget.UsedAmount)
	budget.UpdateTs = model.GetCurrentUtcEpochSecond()
	budget.UpdatedAt = time.Now().In(internal.ProjectTimezone)
	err = model.ProjectBudgetModel.Update(db, budget)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project budget error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// ShowBudgets show project budget records or return empty list
//
//	@summary	Show project budgets
//	@tags		Project
//	@accept		json
//	@produce	json
//	@param		id			path		int				true	"project id"
//	@success	200			{object}	api.Reply
//	@router		/projects/{id}/update_budget [post]
func ShowBudgets(ctx *gin.Context) {
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

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Project Proposals ------ ------

// AddRelatedProposal `POST /projects/:id/add_related_proposal?proposalIDs=1&proposalIDs=2`
//
//	@summary	Add related proposals to the project
//	@router		/projects/:id/add_related_proposal [post]
//	@tags		Project
//	@param		id			path		number		true	"project ID"
//	@param		proposalIDs	query		[]string	true	"proposal ID list"
//	@success	200			{object}	api.Reply
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
	permObject := buildProjectPermObject(id)
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), permObject, internal.ActModify)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, permObject, internal.ActModify)
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	proj, err := model.ProjectModel.Detail(db, uint(id))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project error")))
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
	proj.UpdateTs = model.GetCurrentUtcEpochSecond()
	proj.UpdatedAt = time.Now().In(internal.ProjectTimezone)
	err = model.ProjectModel.CreateOrUpdate(db, proj)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func buildProjectPermObject(projectId int) string {
	return fmt.Sprintf("%s%d", internal.ObjProjPrefix, projectId)
}

func NormalizeWalletAddrInProject(project *model.Project) *model.Project {
	project.Sponsors = lo.Map[string](project.Sponsors, func(wallet string, _ int) string {
		return common.ToFrontendWallet(wallet)
	})
	project.Members = lo.Map[string](project.Members, func(wallet string, _ int) string {
		return common.ToFrontendWallet(wallet)
	})
	for grpName, wallets := range project.GroupedSponsors {
		project.GroupedSponsors[grpName] = lo.Map(wallets, func(wallet string, _ int) string {
			return common.ToFrontendWallet(wallet)
		})
	}

	return project
}

func GenerateProjectBudgetResp(budgetRcds []*model.ProjectBudget) []*ProjectBudgetResp {
	return lo.Map(budgetRcds, func(r *model.ProjectBudget, _ int) *ProjectBudgetResp {
		return &ProjectBudgetResp{
			AssetName:           r.AssetName,
			TotalAmount:         r.TotalAmount.String(),
			UsedAmount:          r.UsedAmount.String(),
			RemainAmount:        r.RemainAmount.String(),
			AdvanceRatio:        r.AdvanceRatio.String(),
			TotalAdvanceAmount:  r.TotalAdvanceAmount.String(),
			UsedAdvanceAmount:   r.UsedAdvanceAmount.String(),
			RemainAdvanceAmount: r.RemainAdvanceAmount.String(),
		}
	})
}
