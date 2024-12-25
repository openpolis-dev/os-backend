package projects_inject

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type ProjectsService struct {
	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`
}

func (s *ProjectsService) Create(ctx *gin.Context, req *CreateReq) (int, *api.Reply) {
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

	user, enforcer, _, _ := api.ForContext(ctx)

	// Check permission, Only CityHall member can create project
	ok, err := enforcer.HasRoleForUser(common.FormatUserWallet(user.Wallet), internal.RoleHall)
	if err != nil {
		log.Error().Msgf("check permission error %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error detail:" + err.Error()))
	}

	if !ok {
		log.Warn().Msgf("permission deny for user %s", common.FormatUserWallet(user.Wallet))
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
		// ctx.JSON(http.StatusForbidden, api.Forbidden())
		return http.StatusForbidden, api.Forbidden()
	}

	tx := s.Db.Begin()
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
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create project error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("create project error detail:" + err.Error()))
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
		_ = model.ProjectBudgetModel.Create(tx, budgets)
	}

	// commit transaction
	tx.Commit()

	// Save logo image to S3
	logoUrl, err := sdk.GetAwsClient().UploadEntityLogo(proj.ID, "project", req.LogoStr)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("upload logo error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("upload logo error detail:" + err.Error()))
	}
	err = s.Db.Model(&proj).Update("logo", logoUrl).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("upload logo error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("upload logo error detail:" + err.Error()))
	}

	// set policies for sponsor user
	err = model.SetWalletPermissionAsProjectSponsor(enforcer, proj.ID, []string{common.FormatUserWallet(user.Wallet)})
	if err != nil {
		log.Error().Msgf("set wallet permission error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("save policy error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("save policy error detail:" + err.Error()))
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

	// ctx.JSON(http.StatusOK, api.Success(proj))
	return http.StatusOK, api.Success(proj)
}

func (s *ProjectsService) Update(ctx *gin.Context, id int, req *UpdateReq) (int, *api.Reply) {
	user, enforcer, _, _ := api.ForContext(ctx)
	proj, err := model.ProjectModel.Detail(s.Db, uint(id))
	if err != nil {
		log.Error().Msgf("get project error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("get project error detail:" + err.Error()))
	}

	if proj == nil {
		log.Error().Msgf("project %d not exist", id)
		sdk.LogUserSideError(ctx, err)
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id)))
		return http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id))
	}

	// check permission, updating some fields require city hall permission
	ok, err := enforcer.HasRoleForUser(common.FormatUserWallet(user.Wallet), internal.RoleHall)
	if err != nil {
		log.Error().Msgf("check permission error %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error detail:" + err.Error()))
	}

	requesterHasCityHallPerm := ok

	sponsorsBeforeUpdate := lo.SliceToMap(proj.Sponsors, func(item string) (string, bool) { return item, true })

	overProject := false
	if (len(req.OverLink) > 0) || (len(req.Sponsors) > 0) {
		// Only city hall can update overlink and sponsors
		if !requesterHasCityHallPerm {
			log.Warn().Msgf("permission deny for user %s", common.FormatUserWallet(user.Wallet))
			sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
			// ctx.JSON(http.StatusForbidden, api.Forbidden())
			return http.StatusForbidden, api.Forbidden()
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
			// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("set sponsor permission error")))
			return http.StatusInternalServerError, api.ServerError(errors.New("set sponsor permission error detail:" + err.Error()))
		} else {
			log.Debug().Msgf("set project sponsor permission for %s", sponsors)
		}

		err = model.RemoveWalletPermissionFromProjectSponsor(enforcer, proj.ID, removedSponsors)
		if err != nil {
			log.Error().Msgf("unset sponsor permission error %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("unset sponsor permission error")))
			return http.StatusInternalServerError, api.ServerError(errors.New("unset sponsor permission error detail:" + err.Error()))
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
					// ctx.JSON(http.StatusForbidden, api.Forbidden())
					return http.StatusForbidden, api.Forbidden()
				}
			} else {
				log.Warn().Msgf("permission deny for user %s", common.FormatUserWallet(user.Wallet))
				sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
				// ctx.JSON(http.StatusForbidden, api.Forbidden())
				return http.StatusForbidden, api.Forbidden()
			}
		}
	}

	// project can be updated only when its status is 'open'
	if proj.Status != model.ProjectStatusOpen {
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d can't be update", id)))
		return http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d can't be update", id))
	}

	logoUrl, err := sdk.GetAwsClient().UploadEntityLogo(proj.ID, "project", req.LogoStr)
	if err != nil {
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("upload logo for project %d failed, err: %+v", id, err)))
		return http.StatusBadRequest, api.BadRequest(fmt.Errorf("upload logo for project %d failed, err: %+v", id, err))
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

	err = model.ProjectModel.CreateOrUpdate(s.Db, proj)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("update project error detail:" + err.Error()))
	}

	// ctx.JSON(http.StatusOK, api.Success(nil))
	return http.StatusOK, api.Success(nil)
}

func (s *ProjectsService) Close(ctx *gin.Context, id int) (int, *api.Reply) {
	user, enforcer, _, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), internal.ObjProj, internal.ActClose)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("check permission error detail:" + err.Error()))
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.ObjProj, internal.ActClose)
		// ctx.JSON(http.StatusForbidden, api.Forbidden())
		return http.StatusForbidden, api.Forbidden()
	}

	project, err := model.ProjectModel.Detail(s.Db, uint(id))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("get project error detail:" + err.Error()))
	}
	if project == nil {
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id)))
		return http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id))
	}

	if project.Status != model.ProjectStatusOpen {
		err := fmt.Errorf("project %d current status %s is not suit for closing", id, project.Status)
		sdk.LogUserSideError(ctx, err)
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return http.StatusBadRequest, api.BadRequest(err)
	}

	err = s.Db.Transaction(func(tx *gorm.DB) error {
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
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("close project failed")))
		return http.StatusInternalServerError, api.ServerError(errors.New("close project failed detail:" + err.Error()))
	}

	// ctx.JSON(http.StatusOK, api.Success(nil))
	return http.StatusOK, api.Success(nil)
}

func (s *ProjectsService) UpdateStaffs(ctx *gin.Context, id int, req *UpdateStaffsReq) (int, *api.Reply) {
	user, enforcer, _, _ := api.ForContext(ctx)
	//  check permission
	if len(req.Sponsors) != 0 {
		permObject := buildProjectPermObject(id)
		ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), permObject, internal.ActUpdateSponsor)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
			return http.StatusInternalServerError, api.ServerError(errors.New("check permission error detail:" + err.Error()))
		}
		if !ok {
			sdk.LogForbiddenError(ctx, user.Wallet, permObject, internal.ActUpdateSponsor)
			// ctx.JSON(http.StatusForbidden, api.Forbidden())
			return http.StatusForbidden, api.Forbidden()
		}
	}
	if len(req.Members) != 0 {
		permObject := buildProjectPermObject(id)
		ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), permObject, internal.ActUpdateMember)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
			return http.StatusInternalServerError, api.ServerError(errors.New("check permission error detail:" + err.Error()))
		}
		if !ok {
			sdk.LogForbiddenError(ctx, user.Wallet, permObject, internal.ActUpdateMember)
			// ctx.JSON(http.StatusForbidden, api.Forbidden())
			return http.StatusForbidden, api.Forbidden()
		}
	}

	// convert all wallet to checksum address
	sponsors := lo.Map[string](req.Sponsors, func(item string, _ int) string {
		return common.FormatUserWallet(item)
	})
	members := lo.Map[string](req.Members, func(item string, _ int) string {
		return common.FormatUserWallet(item)
	})

	proj, err := model.ProjectModel.Detail(s.Db, uint(id))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("get project error detail:" + err.Error()))
	}
	if proj == nil {
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id)))
		return http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id))
	}

	// project can be updated only when its status is 'open'
	if proj.Status != model.ProjectStatusOpen {
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("project can't be update")))
		return http.StatusBadRequest, api.BadRequest(errors.New("project can't be update"))
	}

	// ------ ------ ------ ------ ------ ------ ------ ------ ------

	if req.Action == "add" {
		tx := s.Db.Begin()

		if len(req.Sponsors) != 0 {
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
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
				return http.StatusInternalServerError, api.ServerError(errors.New("update project error detail:" + err.Error()))
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
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
				return http.StatusInternalServerError, api.ServerError(errors.New("update project error detail:" + err.Error()))
			}
			err = enforcer.SavePolicy()
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
				return http.StatusInternalServerError, api.ServerError(errors.New("update project error detail:" + err.Error()))
			}
		}

		if len(req.Members) != 0 {
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
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
				return http.StatusInternalServerError, api.ServerError(errors.New("update project error detail:" + err.Error()))
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
		tx := s.Db.Begin()

		if len(req.Sponsors) != 0 {
			// remove project sponsors
			proj.Sponsors = lo.Without[string](proj.Sponsors, sponsors...)
			err = model.ProjectModel.CreateOrUpdate(tx, proj)
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
				return http.StatusInternalServerError, api.ServerError(errors.New("update project error detail:" + err.Error()))
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
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
				return http.StatusInternalServerError, api.ServerError(errors.New("update project error detail:" + err.Error()))
			}
			err = enforcer.SavePolicy()
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
				return http.StatusInternalServerError, api.ServerError(errors.New("update project error detail:" + err.Error()))
			}
		}

		if len(req.Members) != 0 {
			// remove project members
			proj.Members = lo.Without[string](proj.Members, members...)
			proj.UpdateTs = model.GetCurrentUtcEpochSecond()
			proj.UpdatedAt = time.Now().In(internal.ProjectTimezone)
			err = model.ProjectModel.CreateOrUpdate(tx, proj)
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
				return http.StatusInternalServerError, api.ServerError(errors.New("update project error detail:" + err.Error()))
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

	// ctx.JSON(http.StatusOK, api.Success(nil))
	return http.StatusOK, api.Success(nil)
}

func (s *ProjectsService) UpdateBudget(ctx *gin.Context, id int, req *UpdateBudgetReq) (int, *api.Reply) {
	user, enforcer, _, _ := api.ForContext(ctx)
	//  check permission
	permObject := buildProjectPermObject(id)
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), permObject, internal.ActUpdateBudget)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("check permission error detail:" + err.Error()))
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, permObject, internal.ActUpdateBudget)
		// ctx.JSON(http.StatusForbidden, api.Forbidden())
		return http.StatusForbidden, api.Forbidden()
	}

	proj, err := model.ProjectModel.Detail(s.Db, uint(id))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("get project error detail:" + err.Error()))
	}
	if proj == nil {
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id)))
		return http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id))
	}

	// project can be updated only when its status is 'open'
	if proj.Status != model.ProjectStatusOpen {
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d can't be update", id)))
		return http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d can't be update", id))
	}

	budget, err := model.ProjectBudgetModel.Detail(s.Db, req.Id)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project budget error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("get project budget error detail:" + err.Error()))
	}

	// update `TotalAmount`
	budget.TotalAmount = req.TotalAmount
	budget.RemainAmount = budget.TotalAmount.Sub(budget.UsedAmount)
	budget.UpdateTs = model.GetCurrentUtcEpochSecond()
	budget.UpdatedAt = time.Now().In(internal.ProjectTimezone)
	err = model.ProjectBudgetModel.Update(s.Db, budget)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project budget error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("update project budget error detail:" + err.Error()))
	}

	// ctx.JSON(http.StatusOK, api.Success(nil))
	return http.StatusOK, api.Success(nil)
}

func (s *ProjectsService) AddRelatedProposal(ctx *gin.Context, id int, proposalIDs []string) (int, *api.Reply) {
	user, enforcer, _, _ := api.ForContext(ctx)
	//  check permission
	permObject := buildProjectPermObject(id)
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), permObject, internal.ActModify)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("check permission error detail:" + err.Error()))
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, permObject, internal.ActModify)
		// ctx.JSON(http.StatusForbidden, api.Forbidden())
		return http.StatusForbidden, api.Forbidden()
	}

	proj, err := model.ProjectModel.Detail(s.Db, uint(id))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("get project error detail:" + err.Error()))
	}
	if proj == nil {
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id)))
		return http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id))
	}

	// project can be updated only when its status is 'open'
	if proj.Status != model.ProjectStatusOpen {
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d can't be update", id)))
		return http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d can't be update", id))
	}

	proj.Proposals = append(proj.Proposals, proposalIDs...)
	// remove duplicate proposals
	proj.Proposals = lo.Uniq[string](proj.Proposals)
	proj.UpdateTs = model.GetCurrentUtcEpochSecond()
	proj.UpdatedAt = time.Now().In(internal.ProjectTimezone)
	err = model.ProjectModel.CreateOrUpdate(s.Db, proj)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update project error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("update project error detail:" + err.Error()))
	}

	// ctx.JSON(http.StatusOK, api.Success(nil))
	return http.StatusOK, api.Success(nil)
}
