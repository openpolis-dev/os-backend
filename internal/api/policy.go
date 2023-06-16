package api

const (
	RoleHall = "hall"

	RoleProjSponsorPrefix = "proj_sponsor_"
	//RoleProjMemberPrefix  = "proj_member_"

	RoleGuildSponsorPrefix = "guild_sponsor_"
	//RoleGuildMemberPrefix  = "guild_member_"
)

const (
	ObjProj         = "proj"
	ObjGuild        = "guild"
	ObjProjAndGuild = "proj_and_guild"

	ObjProjPrefix  = "proj_"
	ObjGuildPrefix = "guild_"
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
)

// default policies
//
// `p, hall, *, *` : hall can do anything

// dynamic add policies
//
// `p, proj_sponsor_1, proj_1, modify` :
// `p, proj_sponsor_1, proj_1, create_app` :
// `p, proj_sponsor_1, proj_1, u_member` :
//
// `p, proj_member_1, proj_1, modify` :
// `p, proj_member_1, proj_1, create_app` :
//
// `g, 0xc1ee7cb74583d1509362467443c44f1fca981283, proj_sponsor_1`
// `g, 0xce36c17896adfc975bc1f00e7614c61db05ed376, proj_member_1`

// default roles
//
// `g, 0x183f09c3ce99c02118c570e03808476b22d63191, hall`

/*
hall: can do anything
	- create project/guild
	- close all project/guild
	- modify all project/guild's info(includes: update name+logo, add related proposal)
	- update all project/guild's sponsors and members
	- update all project/guild's budgets
	- audit all project and guild's application
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
	(0x..., proj, create)
	(0x..., proj, close)

	(0x..., proj_and_guild, audit_app)

	(0x..., proj_1, modify)
	(0x..., proj_1, u_sponsor)
	(0x..., proj_1, u_member)
	(0x..., proj_1, u_budget)
	(0x..., proj_1, create_app)
*/
