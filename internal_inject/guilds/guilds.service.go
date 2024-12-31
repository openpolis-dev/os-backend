package guilds_inject

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type GuildsService struct {
	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`
}

func (s *GuildsService) Close(ctx *gin.Context, id int) (int, *api.Reply) {
	user, enforcer, _, _ := api.ForContext(ctx)

	//  check permission
	ok, err := enforcer.HasRoleForUser(common.FormatUserWallet(user.Wallet), internal.RoleHall)
	if err != nil {
		log.Error().Msgf("check permission error %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error")))
		return http.StatusBadRequest, api.BadRequest(errors.New("get cityhall permission error detail:" + err.Error()))
	}

	if !ok {
		log.Warn().Msgf("permission deny for user %s", common.FormatUserWallet(user.Wallet))
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
		// ctx.JSON(http.StatusForbidden, api.Forbidden())
		return http.StatusForbidden, api.Forbidden()
	}

	guild, err := model.GuildModel.Detail(s.Db, uint(id))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get guild error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("get guild error"))
	}
	if guild == nil {
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("guild %d not exist", id)))
		return http.StatusBadRequest, api.BadRequest(fmt.Errorf("guild %d not exist", id))
	}

	if guild.Status != model.ProjectStatusOpen {
		err := fmt.Errorf("guild %d current status %s is not suit for closing", id, guild.Status)
		sdk.LogUserSideError(ctx, err)
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return http.StatusBadRequest, api.BadRequest(err)
	}

	guild.Status = model.ProjectStatusClosed
	guild.UpdateTs = model.GetCurrentUtcEpochSecond()
	guild.UpdatedAt = time.Now().In(internal.ProjectTimezone)
	err = model.GuildModel.CreateOrUpdate(s.Db, guild)

	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("close guild failed")))
		return http.StatusInternalServerError, api.ServerError(errors.New("close guild failed detail:" + err.Error()))
	}

	// ctx.JSON(http.StatusOK, api.Success(nil))
	return http.StatusOK, api.Success(nil)
}

func (s *GuildsService) Create(ctx *gin.Context, req *CreateReq) (int, *api.Reply) {
	// convert all wallet to checksum address
	sponsors := lo.Uniq(lo.Map(req.Sponsors, func(item string, _ int) string {
		return common.FormatUserWallet(item)
	}))
	members := lo.Uniq(lo.Map(req.Members, func(item string, _ int) string {
		return common.FormatUserWallet(item)
	}))
	// remove sponsors from members
	members = lo.Without[string](members, sponsors...)

	// remove duplicate proposals
	proposals := lo.Uniq[string](req.Proposals)

	user, enforcer, _, _ := api.ForContext(ctx)

	// Check permission, Only CityHall member can create guild
	ok, err := enforcer.HasRoleForUser(common.FormatUserWallet(user.Wallet), internal.RoleHall)
	if err != nil {
		log.Error().Msgf("check permission error %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error")))
		return http.StatusBadRequest, api.BadRequest(errors.New("get cityhall permission error detail:" + err.Error()))
	}

	if !ok {
		log.Warn().Msgf("permission deny for user %s", common.FormatUserWallet(user.Wallet))
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
		// ctx.JSON(http.StatusForbidden, api.Forbidden())
		return http.StatusForbidden, api.Forbidden()
	}

	tx := s.Db.Begin()
	// save guild
	guild := model.Guild{
		Name:      req.Name,
		Intro:     req.Intro,
		Desc:      req.Desc,
		Sponsors:  sponsors,
		Members:   members,
		Proposals: proposals,
		Creator:   common.FormatUserWallet(user.Wallet),
		CreateTs:  model.GetCurrentUtcEpochSecond(),
		UpdateTs:  model.GetCurrentUtcEpochSecond(),

		Status: model.ProjectStatusOpen,

		ContantWay:   req.ContantWay,
		OfficialLink: req.OfficialLink,
	}
	err = model.GuildModel.CreateOrUpdate(tx, &guild)
	if err != nil {
		tx.Rollback()
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("save guild error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("save guild error detail:" + err.Error()))
	}

	// commit transaction
	tx.Commit()

	// Save logo image to S3
	logoUrl, err := sdk.GetAwsClient().UploadEntityLogo(guild.ID, "guild", req.LogoStr)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("save logo image error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("save logo image error detail:" + err.Error()))
	}
	err = s.Db.Model(&guild).Update("logo", logoUrl).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("save guild error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("save guild error detail:" + err.Error()))
	}

	// set policies for sponsor user
	err = model.SetWalletPermissionAsGuildSponsor(enforcer, guild.ID, []string{common.FormatUserWallet(user.Wallet)})
	if err != nil {
		log.Error().Msgf("set wallet permission error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("save policy error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("save policy error detail:" + err.Error()))
	}

	// send notification
	push := api.ForContextOnlyPush(ctx)
	staffs := append(sponsors, members...)
	go func(push []sdk.Pusher, staffs []string, guildID uint, guildName string) {
		title, body, data := sdk.GenerateGuildStaffAddNotificationParams(guildID, guildName)
		for _, p := range push {
			err := p.PushToWallets(staffs, title, body, data)
			if err != nil {
				log.Error().Msgf("push to %v failed: %s", staffs, err)
			}
		}
	}(push, staffs, guild.ID, guild.Name)

	// ctx.JSON(http.StatusOK, api.Success(guild))
	return http.StatusOK, api.Success(guild)
}

func (s *GuildsService) Update(ctx *gin.Context, id int, req *UpdateReq) (int, *api.Reply) {
	user, enforcer, _, _ := api.ForContext(ctx)

	// Check permission, Only CityHall member can update guild
	ok, err := enforcer.HasRoleForUser(common.FormatUserWallet(user.Wallet), internal.RoleHall)
	if err != nil {
		log.Error().Msgf("check permission error %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error")))
		return http.StatusBadRequest, api.BadRequest(errors.New("get cityhall permission error detail:" + err.Error()))
	}

	if !ok {
		log.Warn().Msgf("permission deny for user %s", common.FormatUserWallet(user.Wallet))
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
		// ctx.JSON(http.StatusForbidden, api.Forbidden())
		return http.StatusForbidden, api.Forbidden()
	}

	guild, err := model.GuildModel.Detail(s.Db, uint(id))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get guild error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("get guild error"))
	}
	if guild == nil {
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("guild %d not exist", id)))
		return http.StatusBadRequest, api.BadRequest(fmt.Errorf("guild %d not exist", id))
	}

	// Save current sponsors
	sponsorsBeforeUpdate := lo.SliceToMap(guild.Sponsors, func(item string) (string, bool) { return item, true })

	// update logo
	logoUrl, err := sdk.GetAwsClient().UploadEntityLogo(guild.ID, "guild", req.LogoStr)
	if err != nil {
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("upload logo for project %d failed, err: %+v", id, err)))
		return http.StatusBadRequest, api.BadRequest(fmt.Errorf("upload logo for project %d failed, err: %+v", id, err))
	}

	sponsors := lo.Uniq(lo.Map[string](req.Sponsors, func(item string, _ int) string {
		return common.FormatUserWallet(item)
	}))

	// update name
	guild.Logo = logoUrl
	guild.Sponsors = sponsors
	// guild.Name = req.Name
	// guild.Intro = req.Intro
	guild.Desc = req.Desc

	// TODO: Dup code with project update logic
	// Update permissions
	// 1. Grant casbin permission for all existing sponsors
	// 2. Remove casbin permission for all removed sponsors
	removedSponsors := lo.Keys(lo.OmitByKeys(sponsorsBeforeUpdate, sponsors))

	log.Debug().Msgf("set sponsors permission: %+v, removed sponsors: %+v", sponsors, removedSponsors)

	err = model.SetWalletPermissionAsGuildSponsor(enforcer, guild.ID, sponsors)
	if err != nil {
		log.Error().Msgf("set sponsor permission error %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("set sponsor permission error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("set sponsor permission error detail:" + err.Error()))
	} else {
		log.Debug().Msgf("set guild sponsor permission for %s", sponsors)
	}

	err = model.RemoveWalletPermissionFromGuildSponsor(enforcer, guild.ID, removedSponsors)
	if err != nil {
		log.Error().Msgf("unset sponsor permission error %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("unset sponsor permission error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("unset sponsor permission error detail:" + err.Error()))
	} else {
		log.Debug().Msgf("unset project sponsor permission for %s", removedSponsors)
	}

	guild.ContantWay = req.ContantWay
	guild.OfficialLink = req.OfficialLink

	guild.UpdateTs = model.GetCurrentUtcEpochSecond()
	guild.UpdatedAt = time.Now().In(internal.ProjectTimezone)
	err = model.GuildModel.CreateOrUpdate(s.Db, guild)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("update guild error detail:" + err.Error()))
	}

	// ctx.JSON(http.StatusOK, api.Success(nil))
	return http.StatusOK, api.Success(nil)
}

func (s *GuildsService) UpdateStaffs(ctx *gin.Context, id int, req *UpdateStaffsReq) (int, *api.Reply) {
	user, enforcer, _, _ := api.ForContext(ctx)
	//  check permission
	if len(req.Sponsors) != 0 {
		permObject := buildGuildPermObject(id)
		ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), permObject, internal.ActUpdateSponsor)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list guilds error")))
			return http.StatusInternalServerError, api.ServerError(errors.New("list guilds error"))
		}
		if !ok {
			sdk.LogForbiddenError(ctx, user.Wallet, permObject, internal.ActUpdateSponsor)
			// ctx.JSON(http.StatusForbidden, api.Forbidden())
			return http.StatusForbidden, api.Forbidden()
		}
	}
	if len(req.Members) != 0 {
		permObject := buildGuildPermObject(id)
		ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), permObject, internal.ActUpdateMember)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list guilds error")))
			return http.StatusInternalServerError, api.ServerError(errors.New("list guilds error"))
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

	guild, err := model.GuildModel.Detail(s.Db, uint(id))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get guild error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("get guild error"))
	}
	if guild == nil {
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("guild %d not exist", id)))
		return http.StatusBadRequest, api.BadRequest(fmt.Errorf("guild %d not exist", id))
	}

	// ------ ------ ------ ------ ------ ------ ------ ------ ------

	if req.Action == "add" {
		tx := s.Db.Begin()

		if len(req.Sponsors) != 0 {
			// add guild sponsors
			guild.Sponsors = append(guild.Sponsors, sponsors...)
			// remove duplicate sponsors
			guild.Sponsors = lo.Uniq[string](guild.Sponsors)
			// remove sponsors from members
			guild.Sponsors = lo.Without[string](guild.Sponsors, guild.Members...)
			guild.UpdateTs = model.GetCurrentUtcEpochSecond()
			guild.UpdatedAt = time.Now().In(internal.ProjectTimezone)
			err = model.GuildModel.CreateOrUpdate(tx, guild)
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
				return http.StatusInternalServerError, api.ServerError(errors.New("update guild error detail:" + err.Error()))
			}

			// add roles for new sponsors
			newSponsorGroupingPolicies := lo.Map(req.Sponsors, func(sponsor string, _ int) []string {
				// g, 0xc13..1283 guild_sponsor_1
				return []string{common.FormatUserWallet(sponsor), fmt.Sprintf("%s%d", internal.RoleGuildSponsorPrefix, guild.ID)}
			})
			_, err = enforcer.AddGroupingPolicies(newSponsorGroupingPolicies)
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
				return http.StatusInternalServerError, api.ServerError(errors.New("update guild error detail:" + err.Error()))
			}
			err = enforcer.SavePolicy()
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
				return http.StatusInternalServerError, api.ServerError(errors.New("update guild error detail:" + err.Error()))
			}
		}

		if len(req.Members) != 0 {
			// add guild members
			guild.Members = append(guild.Members, members...)
			// remove duplicate members
			guild.Members = lo.Uniq[string](guild.Members)
			// remove members from sponsors
			guild.Members = lo.Without[string](guild.Members, guild.Sponsors...)
			guild.UpdateTs = model.GetCurrentUtcEpochSecond()
			guild.UpdatedAt = time.Now().In(internal.ProjectTimezone)
			err = model.GuildModel.CreateOrUpdate(tx, guild)
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
				return http.StatusInternalServerError, api.ServerError(errors.New("update guild error detail:" + err.Error()))
			}

			//// add roles for new members
			//newMemberGroupingPolicies := lo.Map(req.Members, func(member string, _ int) []string {
			//	// g, 0xc13..1283 guild_member_1
			//	return []string{strings.ToLower(member), fmt.Sprintf("%s%d", api.RoleGuildMemberPrefix, guild.ID)}
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
		go func(push []sdk.Pusher, staffs []string, guildID uint, guildName string) {
			title, body, data := sdk.GenerateGuildStaffAddNotificationParams(guildID, guildName)
			for _, p := range push {
				err := p.PushToWallets(staffs, title, body, data)
				if err != nil {
					log.Error().Msgf("push to %+v failed: %s", staffs, err)
				}
			}
		}(push, staffs, guild.ID, guild.Name)
	} else if req.Action == "remove" {
		tx := s.Db.Begin()

		if len(req.Sponsors) != 0 {
			// remove guild sponsors
			guild.Sponsors = lo.Without[string](guild.Sponsors, sponsors...)
			guild.UpdateTs = model.GetCurrentUtcEpochSecond()
			guild.UpdatedAt = time.Now().In(internal.ProjectTimezone)
			err = model.GuildModel.CreateOrUpdate(tx, guild)
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
				return http.StatusInternalServerError, api.ServerError(errors.New("update guild error detail:" + err.Error()))
			}

			// remove roles for old sponsors
			oldSponsorGroupingPolicies := lo.Map(guild.Sponsors, func(sponsor string, _ int) []string {
				// g, 0xc13..1283 guild_sponsor_1
				return []string{sponsor, fmt.Sprintf("%s%d", internal.RoleGuildSponsorPrefix, guild.ID)}
			})
			_, err = enforcer.RemoveGroupingPolicies(oldSponsorGroupingPolicies)
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
				return http.StatusInternalServerError, api.ServerError(errors.New("update guild error detail:" + err.Error()))
			}
			err = enforcer.SavePolicy()
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
				return http.StatusInternalServerError, api.ServerError(errors.New("update guild error detail:" + err.Error()))
			}
		}

		if len(req.Members) != 0 {
			// remove guild members
			guild.Members = lo.Without[string](guild.Members, members...)
			guild.UpdateTs = model.GetCurrentUtcEpochSecond()
			guild.UpdatedAt = time.Now().In(internal.ProjectTimezone)
			err = model.GuildModel.CreateOrUpdate(tx, guild)
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
				return http.StatusInternalServerError, api.ServerError(errors.New("update guild error detail:" + err.Error()))
			}

			//// remove roles for old members
			//oldMemberGroupingPolicies := lo.Map(guild.Members, func(member string, _ int) []string {
			//	// g, 0xc13..1283 guild_member_1
			//	return []string{member, fmt.Sprintf("%s%d", api.RoleGuildMemberPrefix, guild.ID)}
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
		go func(push []sdk.Pusher, staffs []string, guildID uint, guildName string) {
			title, body, data := sdk.GenerateGuildStaffRemoveNotificationParams(guildID, guildName)
			for _, p := range push {
				err := p.PushToWallets(staffs, title, body, data)
				if err != nil {
					log.Error().Msgf("push to %+v failed: %s", staffs, err)
				}
			}
		}(push, staffs, guild.ID, guild.Name)
	}

	// ------ ------ ------ ------ ------ ------ ------ ------ ------

	// ctx.JSON(http.StatusOK, api.Success(nil))
	return http.StatusOK, api.Success(nil)
}

func (s *GuildsService) UpdateBudget(ctx *gin.Context, id int, req *UpdateBudgetReq) (int, *api.Reply) {
	user, enforcer, _, _ := api.ForContext(ctx)
	//  check permission
	permObject := buildGuildPermObject(id)
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), permObject, internal.ActUpdateBudget)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return http.StatusBadRequest, api.BadRequest(errors.New("check permission error detail:" + err.Error()))
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, permObject, internal.ActModify)
		// ctx.JSON(http.StatusForbidden, api.Forbidden())
		return http.StatusForbidden, api.Forbidden()
	}

	budget, err := model.GuildBudgetModel.Detail(s.Db, req.Id)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get guild budget error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("get guild budget error"))
	}
	// update `TotalAmount`
	budget.TotalAmount = req.TotalAmount
	err = model.GuildBudgetModel.Update(s.Db, budget)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild budget error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("update guild budget error detail:" + err.Error()))
	}

	// ctx.JSON(http.StatusOK, api.Success(nil))
	return http.StatusOK, api.Success(nil)
}

func (s *GuildsService) AddRelatedProposal(ctx *gin.Context, id int, proposalIDs []string) (int, *api.Reply) {
	user, enforcer, _, _ := api.ForContext(ctx)
	//  check permission
	permObject := buildGuildPermObject(id)
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), permObject, internal.ActModify)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return http.StatusBadRequest, api.BadRequest(errors.New("check permission error detail:" + err.Error()))
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, permObject, internal.ActModify)
		// ctx.JSON(http.StatusForbidden, api.Forbidden())
		return http.StatusForbidden, api.Forbidden()
	}

	guild, err := model.GuildModel.Detail(s.Db, uint(id))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get guild error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("get guild error"))
	}
	if guild == nil {
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("guild %d not exist", id)))
		return http.StatusBadRequest, api.BadRequest(fmt.Errorf("guild %d not exist", id))
	}

	guild.Proposals = append(guild.Proposals, proposalIDs...)
	// remove duplicate proposals
	guild.Proposals = lo.Uniq[string](guild.Proposals)
	guild.UpdateTs = model.GetCurrentUtcEpochSecond()
	guild.UpdatedAt = time.Now().In(internal.ProjectTimezone)
	err = model.GuildModel.CreateOrUpdate(s.Db, guild)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("update guild error detail:" + err.Error()))
	}

	// ctx.JSON(http.StatusOK, api.Success(nil))
	return http.StatusOK, api.Success(nil)
}
