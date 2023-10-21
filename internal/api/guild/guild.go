package guild

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
)

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Guild ------ ------

type (
	CreateReq struct {
		LogoStr string `json:"logo"` // base64 encoded logo image, will be uploaded to AWS S3 and saved URL in db record
		Name    string `json:"name"`

		Sponsors  []string `json:"sponsors"`
		Members   []string `json:"members"`
		Proposals []string `json:"proposals"`

		Budgets []*BudgetParam `json:"budgets"`
	}
	BudgetParam struct {
		Name        string           `json:"name"`
		BudgetType  model.BudgetType `json:"budget_type"`
		TotalAmount decimal.Decimal  `json:"total_amount"`
	}
	UpdateReq struct {
		LogoStr string `json:"logo"`
		Name    string `json:"name"`
	}
	DetailReply struct {
		model.Guild
		Budgets []*model.GuildBudget `json:"budgets"`
	}
	UpdateSponsorsReq struct {
		Sponsors []string `json:"sponsors"`
	}
	UpdateMembersReq struct {
		Members []string `json:"members"`
	}
	UpdateBudgetReq struct {
		Id          uint            `json:"id"`
		AssetName   string          `json:"asset_name"`
		TotalAmount decimal.Decimal `json:"total_amount"`
	}
)

// Create `POST /guilds`
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

	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(user.Wallet, api.ObjGuild, api.ActCreate)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	tx := db.Begin()
	// save guild
	guild := model.Guild{
		Name:      req.Name,
		Sponsors:  sponsors,
		Members:   members,
		Proposals: req.Proposals,
	}
	err = model.GuildModel.CreateOrUpdate(tx, &guild)
	if err != nil {
		tx.Rollback()
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	// save guild budgets
	budgets := lo.Map[*BudgetParam, *model.GuildBudget](req.Budgets, func(item *BudgetParam, _ int) *model.GuildBudget {
		return &model.GuildBudget{
			GuildID:      guild.ID,
			Type:         item.BudgetType,
			Name:         item.Name,
			TotalAmount:  item.TotalAmount,
			RemainAmount: item.TotalAmount,
		}
	})
	err = model.GuildBudgetModel.Create(tx, budgets)
	if err != nil {
		tx.Rollback()
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	// commit transaction
	tx.Commit()

	// Save logo image to S3
	logoUrl, err := sdk.GetAwsClient().UploadEntityLogo(guild.ID, "guild", req.LogoStr)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	err = db.Model(&guild).Update("logo", logoUrl).Error
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	// add policies
	policies := [][]string{
		// p, guild_sponsor_1, guild_1, modify
		// p, guild_sponsor_1, guild_1, create_app
		// p, guild_sponsor_1, guild_1, u_member
		// p, guild_sponsor_1, guild_1, u_budget
		{fmt.Sprintf("%s%d", api.RoleGuildSponsorPrefix, guild.ID), fmt.Sprintf("%s%d", api.ObjGuildPrefix, guild.ID), api.ActModify},
		{fmt.Sprintf("%s%d", api.RoleGuildSponsorPrefix, guild.ID), fmt.Sprintf("%s%d", api.ObjGuildPrefix, guild.ID), api.ActCreateApplication},
		{fmt.Sprintf("%s%d", api.RoleGuildSponsorPrefix, guild.ID), fmt.Sprintf("%s%d", api.ObjGuildPrefix, guild.ID), api.ActUpdateMember},
		{fmt.Sprintf("%s%d", api.RoleGuildSponsorPrefix, guild.ID), fmt.Sprintf("%s%d", api.ObjGuildPrefix, guild.ID), api.ActUpdateBudget},
		//// p, guild_member_1, guild_1, modify
		//// p, guild_member_1, guild_1, create_app
		//{fmt.Sprintf("%s%d", api.RoleGuildMemberPrefix, guild.ID), fmt.Sprintf("%s%d", api.ObjGuildPrefix, guild.ID), api.ActModify},
		//{fmt.Sprintf("%s%d", api.RoleGuildMemberPrefix, guild.ID), fmt.Sprintf("%s%d", api.ObjGuildPrefix, guild.ID), api.ActCreateApplication},
	}
	_, err = enforcer.AddPolicies(policies)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	// add roles
	sponsorGroupingPolicies := lo.Map(req.Sponsors, func(sponsor string, _ int) []string {
		// g, 0xc13..1283 guild_sponsor_1
		return []string{strings.ToLower(sponsor), fmt.Sprintf("%s%d", api.RoleGuildSponsorPrefix, guild.ID)}
	})
	//memberGroupingPolicies := lo.Map(req.Members, func(member string, _ int) []string {
	//	// g, 0xc13..1283 guild_member_1
	//	return []string{strings.ToLower(member), fmt.Sprintf("%s%d", api.RoleGuildMemberPrefix, guild.ID)}
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
	go func(push *sdk.Push, staffs []string, guildID uint, guildName string) {
		title, body, data := api.GenerateGuildStaffAddNotificationParams(guildID, guildName)
		err := push.PushToWallets(staffs, title, body, data)
		if err != nil {
			log.Error().Msgf("push to %v failed: %s", staffs, err)
		}
	}(push, staffs, guild.ID, guild.Name)

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// Update `PUT /guilds/:id`
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
	ok, err := enforcer.Enforce(user.Wallet, fmt.Sprintf("%s%d", api.ObjGuildPrefix, id), api.ActModify)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	guild, err := model.GuildModel.Detail(db, uint(id))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if guild == nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("guild %d not exist", id)))
		return
	}

	// update logo
	logoUrl, err := sdk.GetAwsClient().UploadEntityLogo(guild.ID, "guild", req.LogoStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("upload logo for project %d failed, err: %+v", id, err)))
		return
	}

	// update name
	guild.Logo = logoUrl
	guild.Name = req.Name
	err = model.GuildModel.CreateOrUpdate(db, guild)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// Detail `GET /guilds/:id`
func Detail(ctx *gin.Context) {
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	db := api.ForContextOnlyDB(ctx)

	guild, err := model.GuildModel.Detail(db, uint(id))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if guild == nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("guild %d not exist", id)))
		return
	}

	budgets, err := model.GuildBudgetModel.ListByGuildId(db, guild.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&DetailReply{
		Guild:   *guild,
		Budgets: budgets,
	}))
}

// List `GET /guilds?page=1&size=10&sort_field=created_at&sort_order=desc`
func List(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)

	page := api.ParseAndConvertPageParam(ctx)

	guilds, total, err := model.GuildModel.List(db, page)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  page.Page,
		Size:  page.Size,
		Total: total,
		Rows:  guilds,
	}))
}

// MyGuilds `GET /guilds/my?page=1&size=10&sort_field=created_at&sort_order=desc`
func MyGuilds(ctx *gin.Context) {
	user, db := api.ForContextUserAndDB(ctx)

	page := api.ParseAndConvertPageParam(ctx)

	guilds, total, err := model.GuildModel.ListBySponsorOrMember(db, user.Wallet, page)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  page.Page,
		Size:  page.Size,
		Total: total,
		Rows:  guilds,
	}))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Guild Sponsors/Members ------ ------

type UpdateStaffsReq struct {
	Action   string   `json:"action"` // `add` or `remove`
	Sponsors []string `json:"sponsors"`
	Members  []string `json:"members"`
}

// UpdateStaffs `POST /guilds/:id/update_staffs`
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
		ok, err := enforcer.Enforce(user.Wallet, fmt.Sprintf("%s%d", api.ObjGuildPrefix, id), api.ActUpdateSponsor)
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
		ok, err := enforcer.Enforce(user.Wallet, fmt.Sprintf("%s%d", api.ObjGuildPrefix, id), api.ActUpdateMember)
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

	guild, err := model.GuildModel.Detail(db, uint(id))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if guild == nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("guild %d not exist", id)))
		return
	}

	// ------ ------ ------ ------ ------ ------ ------ ------ ------

	if req.Action == "add" {
		tx := db.Begin()

		if req.Sponsors != nil && len(req.Sponsors) != 0 {
			// add guild sponsors
			guild.Sponsors = append(guild.Sponsors, sponsors...)
			// remove duplicate sponsors
			guild.Sponsors = lo.Uniq[string](guild.Sponsors)
			err = model.GuildModel.CreateOrUpdate(tx, guild)
			if err != nil {
				tx.Rollback()

				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
				return
			}

			// add roles for new sponsors
			newSponsorGroupingPolicies := lo.Map(req.Sponsors, func(sponsor string, _ int) []string {
				// g, 0xc13..1283 guild_sponsor_1
				return []string{strings.ToLower(sponsor), fmt.Sprintf("%s%d", api.RoleGuildSponsorPrefix, guild.ID)}
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
			// add guild members
			guild.Members = append(guild.Members, members...)
			// remove duplicate members
			guild.Members = lo.Uniq[string](guild.Members)
			err = model.GuildModel.CreateOrUpdate(tx, guild)
			if err != nil {
				tx.Rollback()

				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
				return
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
		go func(push *sdk.Push, staffs []string, guildID uint, guildName string) {
			title, body, data := api.GenerateGuildStaffAddNotificationParams(guildID, guildName)
			err := push.PushToWallets(staffs, title, body, data)
			if err != nil {
				log.Error().Msgf("push to %+v failed: %s", staffs, err)
			}
		}(push, staffs, guild.ID, guild.Name)
	} else if req.Action == "remove" {
		tx := db.Begin()

		if req.Sponsors != nil && len(req.Sponsors) != 0 {
			// remove guild sponsors
			guild.Sponsors = lo.Without[string](guild.Sponsors, sponsors...)
			err = model.GuildModel.CreateOrUpdate(tx, guild)
			if err != nil {
				tx.Rollback()

				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
				return
			}

			// remove roles for old sponsors
			oldSponsorGroupingPolicies := lo.Map(guild.Sponsors, func(sponsor string, _ int) []string {
				// g, 0xc13..1283 guild_sponsor_1
				return []string{sponsor, fmt.Sprintf("%s%d", api.RoleGuildSponsorPrefix, guild.ID)}
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
			// remove guild members
			guild.Members = lo.Without[string](guild.Members, members...)
			err = model.GuildModel.CreateOrUpdate(tx, guild)
			if err != nil {
				tx.Rollback()

				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
				return
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
		go func(push *sdk.Push, staffs []string, guildID uint, guildName string) {
			title, body, data := api.GenerateGuildStaffRemoveNotificationParams(guildID, guildName)
			err := push.PushToWallets(staffs, title, body, data)
			if err != nil {
				log.Error().Msgf("push to %+v failed: %s", staffs, err)
			}
		}(push, staffs, guild.ID, guild.Name)
	}

	// ------ ------ ------ ------ ------ ------ ------ ------ ------

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Guild Budget ------ ------

// UpdateBudget `POST /guilds/:id/update_budget`
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
	ok, err := enforcer.Enforce(user.Wallet, fmt.Sprintf("%s%d", api.ObjGuildPrefix, id), api.ActUpdateBudget)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	budget, err := model.GuildBudgetModel.Detail(db, req.Id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	// update `TotalAmount`
	budget.TotalAmount = req.TotalAmount
	err = model.GuildBudgetModel.Update(db, budget)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Guild Proposals ------ ------

// AddRelatedProposal `POST /guilds/:id/add_related_proposal?proposalIDs=1&proposalIDs=2`
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
	ok, err := enforcer.Enforce(user.Wallet, fmt.Sprintf("%s%d", api.ObjGuildPrefix, id), api.ActModify)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	guild, err := model.GuildModel.Detail(db, uint(id))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if guild == nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("guild %d not exist", id)))
		return
	}

	guild.Proposals = append(guild.Proposals, proposalIDs...)
	// remove duplicate proposals
	guild.Proposals = lo.Uniq[string](guild.Proposals)
	err = model.GuildModel.CreateOrUpdate(db, guild)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}
