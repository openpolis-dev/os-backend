package guild

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
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
// ------ Guild ------ ------

type (
	CreateReq struct {
		LogoStr string `json:"logo"` // base64 encoded logo image, will be uploaded to AWS S3 and saved URL in db record
		Name    string `json:"name"`
		Intro   string `json:"intro"`
		Desc    string `json:"desc"`

		Sponsors  []string `json:"sponsors"`
		Members   []string `json:"members"`
		Proposals []string `json:"proposals"`

		Budgets []*BudgetParam `json:"budgets"`

		ContantWay   string `json:"ContantWay"`
		OfficialLink string `json:"OfficialLink"`
	}
	BudgetParam struct {
		Name        string          `json:"name"`
		TotalAmount decimal.Decimal `json:"total_amount"`
	}
	UpdateReq struct {
		LogoStr string `json:"logo"`
		// Name    string `json:"name"`
		// Intro   string `json:"intro"`
		Desc string `json:"desc"`

		ContantWay   string `json:"ContantWay"`
		OfficialLink string `json:"OfficialLink"`
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

// Close
// POST /guild/:id/close
//
//	@summary		Close a project
//	@description	This api close specified project, admin permission is required for this operation
//	@router			/guild/:id/close [post]
//	@tags			Guild
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
	// ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), internal.ObjProj, internal.ActClose)
	// if err != nil {
	// 	sdk.LogServerErrorToSentry(ctx, err)
	// 	ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
	// 	return
	// }
	// if !ok {
	// 	sdk.LogForbiddenError(ctx, user.Wallet, internal.ObjProj, internal.ActClose)
	// 	ctx.JSON(http.StatusForbidden, api.Forbidden())
	// 	return
	// }

	//  check permission
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

	// project, err := model.ProjectModel.Detail(db, uint(id))
	// if err != nil {
	// 	sdk.LogServerErrorToSentry(ctx, err)
	// 	ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project error")))
	// 	return
	// }
	// if project == nil {
	// 	ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("project %d not exist", id)))
	// 	return
	// }

	if guild.Status != model.ProjectStatusOpen {
		err := fmt.Errorf("guild %d current status %s is not suit for closing", id, guild.Status)
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
			EntityType: "guild",
			EntityId:   guild.ID,
		}
		err = model.NewApplicationRecord(tx, &application)
		if err != nil {
			return err
		}
		guild.Status = model.ProjectStatusPendingClose
		guild.UpdatedAt = time.Now().In(internal.ProjectTimezone)
		guild.UpdateTs = model.GetCurrentUtcEpochSecond()
		return tx.Save(guild).Error
	})

	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("close guild failed")))
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// Create a guild
//
//	@summary	Create a guild
//	@tags		Guild
//	@accept		json
//	@produce	json
//	@param		JsonBody	body		CreateReq	true	"request json body"
//	@success	200			{object}	api.Reply
//	@router		/guilds [post]
func Create(ctx *gin.Context) {
	req := CreateReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
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
	//  check permission
	// ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), internal.ObjGuild, internal.ActCreate)
	// if err != nil {
	// 	sdk.LogServerErrorToSentry(ctx, err)
	// 	ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
	// 	return
	// }
	// if !ok {
	// 	sdk.LogForbiddenError(ctx, user.Wallet, internal.ObjGuild, internal.ActCreate)
	// 	ctx.JSON(http.StatusForbidden, api.Forbidden())
	// 	return
	// }

	//  check permission
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
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("save guild error")))
		return
	}

	// commit transaction
	tx.Commit()

	// Save logo image to S3
	logoUrl, err := sdk.GetAwsClient().UploadEntityLogo(guild.ID, "guild", req.LogoStr)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("save logo image error")))
		return
	}
	err = db.Model(&guild).Update("logo", logoUrl).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("save guild error")))
		return
	}

	// add policies
	policies := [][]string{
		// p, guild_sponsor_1, guild_1, modify
		// p, guild_sponsor_1, guild_1, create_app
		// p, guild_sponsor_1, guild_1, u_member
		// p, guild_sponsor_1, guild_1, u_budget
		{fmt.Sprintf("%s%d", internal.RoleGuildSponsorPrefix, guild.ID), fmt.Sprintf("%s%d", internal.ObjGuildPrefix, guild.ID), internal.ActModify},
		{fmt.Sprintf("%s%d", internal.RoleGuildSponsorPrefix, guild.ID), fmt.Sprintf("%s%d", internal.ObjGuildPrefix, guild.ID), internal.ActCreateApplication},
		{fmt.Sprintf("%s%d", internal.RoleGuildSponsorPrefix, guild.ID), fmt.Sprintf("%s%d", internal.ObjGuildPrefix, guild.ID), internal.ActUpdateMember},
		{fmt.Sprintf("%s%d", internal.RoleGuildSponsorPrefix, guild.ID), fmt.Sprintf("%s%d", internal.ObjGuildPrefix, guild.ID), internal.ActUpdateBudget},
		//// p, guild_member_1, guild_1, modify
		//// p, guild_member_1, guild_1, create_app
		//{fmt.Sprintf("%s%d", api.RoleGuildMemberPrefix, guild.ID), fmt.Sprintf("%s%d", api.ObjGuildPrefix, guild.ID), api.ActModify},
		//{fmt.Sprintf("%s%d", api.RoleGuildMemberPrefix, guild.ID), fmt.Sprintf("%s%d", api.ObjGuildPrefix, guild.ID), api.ActCreateApplication},
	}
	_, err = enforcer.AddPolicies(policies)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("add policies error")))
		return
	}
	// add roles
	sponsorGroupingPolicies := lo.Map(req.Sponsors, func(sponsor string, _ int) []string {
		// g, 0xc13..1283 guild_sponsor_1
		return []string{common.FormatUserWallet(sponsor), fmt.Sprintf("%s%d", internal.RoleGuildSponsorPrefix, guild.ID)}
	})
	//memberGroupingPolicies := lo.Map(req.Members, func(member string, _ int) []string {
	//	// g, 0xc13..1283 guild_member_1
	//	return []string{strings.ToLower(member), fmt.Sprintf("%s%d", api.RoleGuildMemberPrefix, guild.ID)}
	//})
	//groupingPolicies := append(memberGroupingPolicies, sponsorGroupingPolicies...)
	_, err = enforcer.AddGroupingPolicies(sponsorGroupingPolicies)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("add grouping policies error")))
		return
	}
	err = enforcer.SavePolicy()
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("save policy error")))
		return
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

	ctx.JSON(http.StatusOK, api.Success(guild))
}

// Update a guild
//
//	@summary	Update a guild
//	@tags		Guild
//	@accept		json
//	@produce	json
//	@param		id			path		int			true	"guild id"
//	@param		JsonBody	body		UpdateReq	true	"request json body"
//	@success	200			{object}	api.Reply
//	@router		/guilds/{id} [put]
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
	// permObject := buildGuildPermObject(id)
	// ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), permObject, internal.ActModify)
	// if err != nil {
	// 	sdk.LogServerErrorToSentry(ctx, err)
	// 	ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
	// 	return
	// }
	// if !ok {
	// 	sdk.LogForbiddenError(ctx, user.Wallet, permObject, internal.ActModify)
	// 	ctx.JSON(http.StatusForbidden, api.Forbidden())
	// 	return
	// }

	//  check permission
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

	// update logo
	logoUrl, err := sdk.GetAwsClient().UploadEntityLogo(guild.ID, "guild", req.LogoStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("upload logo for project %d failed, err: %+v", id, err)))
		return
	}

	// update name
	guild.Logo = logoUrl
	// guild.Name = req.Name
	// guild.Intro = req.Intro
	guild.Desc = req.Desc

	guild.ContantWay = req.ContantWay
	guild.OfficialLink = req.OfficialLink

	guild.UpdateTs = model.GetCurrentUtcEpochSecond()
	guild.UpdatedAt = time.Now().In(internal.ProjectTimezone)
	err = model.GuildModel.CreateOrUpdate(db, guild)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// Detail get a guild detail
//
//	@summary	Get a guild detail
//	@tags		Guild
//	@accept		json
//	@produce	json
//	@param		id	path		int	true	"guild id"
//	@success	200	{object}	api.Reply{data=DetailReply}
//	@router		/guilds/{id} [get]
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

// List `GET /guilds?page=1&size=10&sort_field=create_ts&sort_order=desc`
//
//	@summary	List guilds
//	@tags		Guild
//	@accept		json
//	@produce	json
//	@param		page		query		int		false	"page number, default: 1"
//	@param		size		query		int		false	"page size, default: 10"
//	@param		sort_field	query		string	false	"sort field, default: create_ts"
//	@param		sort_order	query		string	false	"sort order, default: desc"
//	@success	200			{object}	api.Reply{data=api.ListReplyData{rows=model.Guild}}
//	@router		/guilds [get]
func List(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)

	page := api.ParseAndConvertPageParam(ctx)

	guilds, total, err := model.GuildModel.List(db, page)
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

// MyGuilds list my guilds
//
//	`GET /guilds/my?page=1&size=10&sort_field=create_ts&sort_order=desc`
//
//	@summary	list my guilds
//	@tags		Guild
//	@accept		json
//	@produce	json
//	@param		page		query		int		false	"page number, default: 1"
//	@param		size		query		int		false	"page size, default: 10"
//	@param		sort_field	query		string	false	"sort field, default: create_ts"
//	@param		sort_order	query		string	false	"sort order, default: desc"
//	@success	200			{object}	api.Reply{data=api.ListReplyData{rows=model.Guild}}
//	@router		/guilds/my_guilds [get]
func MyGuilds(ctx *gin.Context) {
	user, db := api.ForContextUserAndDB(ctx)

	page := api.ParseAndConvertPageParam(ctx)

	guilds, total, err := model.GuildModel.ListBySponsorOrMember(db, common.FormatUserWallet(user.Wallet), page)
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

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Guild Sponsors/Members ------ ------

type UpdateStaffsReq struct {
	Action   string   `json:"action"` // `add` or `remove`
	Sponsors []string `json:"sponsors"`
	Members  []string `json:"members"`
}

// UpdateStaffs update guild sponsors/members
//
//	@summary	Update guild sponsors/members
//	@tags		Guild
//	@accept		json
//	@produce	json
//	@param		id			path		int				true	"guild id"
//	@param		JsonBody	body		UpdateStaffsReq	true	"request json body"
//	@success	200			{object}	api.Reply
//	@router		/guilds/{id}/update_staffs [post]
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
		permObject := buildGuildPermObject(id)
		ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), permObject, internal.ActUpdateSponsor)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list guilds error")))
			return
		}
		if !ok {
			sdk.LogForbiddenError(ctx, user.Wallet, permObject, internal.ActUpdateSponsor)
			ctx.JSON(http.StatusForbidden, api.Forbidden())
			return
		}
	}
	if req.Members != nil && len(req.Members) != 0 {
		permObject := buildGuildPermObject(id)
		ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), permObject, internal.ActUpdateMember)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list guilds error")))
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

	// ------ ------ ------ ------ ------ ------ ------ ------ ------

	if req.Action == "add" {
		tx := db.Begin()

		if req.Sponsors != nil && len(req.Sponsors) != 0 {
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
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
				return
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
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
				return
			}
			err = enforcer.SavePolicy()
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
				return
			}
		}

		if req.Members != nil && len(req.Members) != 0 {
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
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
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
		tx := db.Begin()

		if req.Sponsors != nil && len(req.Sponsors) != 0 {
			// remove guild sponsors
			guild.Sponsors = lo.Without[string](guild.Sponsors, sponsors...)
			guild.UpdateTs = model.GetCurrentUtcEpochSecond()
			guild.UpdatedAt = time.Now().In(internal.ProjectTimezone)
			err = model.GuildModel.CreateOrUpdate(tx, guild)
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
				return
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
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
				return
			}
			err = enforcer.SavePolicy()
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
				return
			}
		}

		if req.Members != nil && len(req.Members) != 0 {
			// remove guild members
			guild.Members = lo.Without[string](guild.Members, members...)
			guild.UpdateTs = model.GetCurrentUtcEpochSecond()
			guild.UpdatedAt = time.Now().In(internal.ProjectTimezone)
			err = model.GuildModel.CreateOrUpdate(tx, guild)
			if err != nil {
				tx.Rollback()

				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
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

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Guild Budget ------ ------

// UpdateBudget update guild budget
//
//	@summary	Update guild budget
//	@tags		Guild
//	@accept		json
//	@produce	json
//	@param		id			path		int				true	"guild id"
//	@param		JsonBody	body		UpdateBudgetReq	true	"request json body"
//	@success	200			{object}	api.Reply
//	@router		/guilds/{id}/update_budget [post]
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
	permObject := buildGuildPermObject(id)
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), permObject, internal.ActUpdateBudget)
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

	budget, err := model.GuildBudgetModel.Detail(db, req.Id)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get guild budget error")))
		return
	}
	// update `TotalAmount`
	budget.TotalAmount = req.TotalAmount
	err = model.GuildBudgetModel.Update(db, budget)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild budget error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ Guild Proposals ------ ------

// AddRelatedProposal add related proposals to guild
//
//	`POST /guilds/:id/add_related_proposal?proposalIDs=1&proposalIDs=2`
//
//	@summary	Add related proposals to guild
//	@tags		Guild
//	@accept		json
//	@produce	json
//	@param		id			path		int		true	"guild id"
//	@param		proposalIDs	query		[]int	true	"proposal ids"
//	@success	200			{object}	api.Reply
//	@router		/guilds/{id}/add_related_proposal [post]
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
	permObject := buildGuildPermObject(id)
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

	guild.Proposals = append(guild.Proposals, proposalIDs...)
	// remove duplicate proposals
	guild.Proposals = lo.Uniq[string](guild.Proposals)
	guild.UpdateTs = model.GetCurrentUtcEpochSecond()
	guild.UpdatedAt = time.Now().In(internal.ProjectTimezone)
	err = model.GuildModel.CreateOrUpdate(db, guild)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update guild error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func buildGuildPermObject(guildId int) string {
	return fmt.Sprintf("%s%d", internal.ObjGuildPrefix, guildId)
}

func NormalizeWalletAddrInGuild(guild *model.Guild) *model.Guild {
	guild.Sponsors = lo.Map[string](guild.Sponsors, func(wallet string, _ int) string {
		return common.ToFrontendWallet(wallet)
	})
	guild.Members = lo.Map[string](guild.Members, func(wallet string, _ int) string {
		return common.ToFrontendWallet(wallet)
	})

	return guild
}
