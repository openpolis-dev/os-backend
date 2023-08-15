package api

const (
	RoleHall = "hall"

	RoleProjSponsorPrefix = "proj_sponsor_"
	//RoleProjMemberPrefix  = "proj_member_"

	RoleGuildSponsorPrefix = "guild_sponsor_"
	//RoleGuildMemberPrefix  = "guild_member_"

	RoleTreasuryManager = "treasury_manager"

	RoleEventManager = "event_manager"
)

const (
	ObjProj         = "proj"
	ObjGuild        = "guild"
	ObjProjAndGuild = "proj_and_guild"

	ObjProjPrefix  = "proj_"
	ObjGuildPrefix = "guild_"

	ObjTreasury = "treasury"

	ObjEvent = "event"
)

const (
	ActCreate = "create"
	ActClose  = "close"

	ActModify            = "modify"
	ActUpdateSponsor     = "u_sponsor"
	ActUpdateMember      = "u_member"
	ActUpdateBudget      = "u_budget"
	ActCreateApplication = "create_app"
	ActAuditApplication  = "audit_app"

	ActUpdateAssertBudget = "u_assert_budget"

	ActCreateEvent = "create_event"
)

var AllProjectActions = []string{ActModify, ActCreateApplication, ActUpdateMember, ActUpdateBudget}

// default policies
//
// `p, hall, *, *` : hall can do anything
// `p, treasury_manager, treasury, u_assert_budget`
// `p, event_manager, event, create_event`

// dynamic add policies
//
// `p, proj_sponsor_1, proj_1, modify` :
// `p, proj_sponsor_1, proj_1, create_app` :
// `p, proj_sponsor_1, proj_1, u_member` :
// `p, proj_sponsor_1, proj_1, u_budget` :
//
// `p, proj_member_1, proj_1, modify` :
// `p, proj_member_1, proj_1, create_app` :
//
// `g, 0xc1ee7cb74583d1509362467443c44f1fca981283, proj_sponsor_1`
// `g, 0xce36c17896adfc975bc1f00e7614c61db05ed376, proj_member_1`
//
// `g, 0x...123, treasury_manager`
//
// `g, 0x...123, event_manager`

// default roles
//
// `g, 0x183f09c3ce99c02118c570e03808476b22d63191, hall`

/*
hall: can do anything
	- create project/guild
	- close all project
	- modify all project/guild's info(includes: update name+logo, add related proposal)
	- update all project/guild's sponsors and members
	- update all project/guild's budgets
	- audit all project and guild's application
	- update seedao assert budget
project/guild sponsor:
	- modify this project/guild's info
	- create application
	- update this project/guild's member
	- update this project/guild's budget
project/guild member: !! project/guild member has no permissions !!
	- modify this project/guild's info
	- create application
*/

/*
	(0x..., proj, create) 创建项目
	(0x..., proj, close)  关闭项目

	(0x..., guild, create) 创建工会


	(0x..., proj_1, modify)     修改项目基本信息
	(0x..., proj_1, u_sponsor)  修改项目牵头人
	(0x..., proj_1, u_member)   修改项目成员
	(0x..., proj_1, u_budget)   修改项目预算
	(0x..., proj_1, create_app) 创建项目的申请

	(0x..., guild_1, modify)     修改工会基本信息
	(0x..., guild_1, u_sponsor)  修改工会牵头人
	(0x..., guild_1, u_member)   修改工会成员
	(0x..., guild_1, u_budget)   修改工会预算
	(0x..., guild_1, create_app) 创建工会的申请


	(0x..., proj_and_guild, audit_app) 审核项目和工会的申请


	(0x..., treasury, u_assert_budget)   修改 SeedAO 的资产预算


	(0x..., event, create_event)  创建/更新活动
*/
