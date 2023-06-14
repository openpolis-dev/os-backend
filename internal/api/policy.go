package api

const (
	RoleHall = "hall"

	RoleProjAdmin  = "proj_admin"
	RoleGuildAdmin = "guild_admin"

	RoleProjSponsorPrefix = "proj_sponsor_"
	RoleProjMemberPrefix  = "proj_member_"

	RoleGuildSponsorPrefix = "guild_sponsor_"
	RoleGuildMemberPrefix  = "guild_member_"
)

const (
	ObjProj  = "proj"
	ObjGuild = "guild"

	ObjProjPrefix  = "proj_"
	ObjGuildPrefix = "guild_"
)

const (
	ActCreate = "create"
	ActModify = "modify"
	ActClose  = "close"
)

// default policies
//
// `p, hall, *, *` : hall can do anything
//
// `p, proj_admin, proj, create` : create project permission
// `p, guild_admin, guild, create` : create guild permission
//
// `p, proj_admin, proj, close` : close project permission
// `p, guild_admin, guild, close` : close guild permission
//
// `p, proj_sponsor_1, proj_1, modify` : role proj_sponsor_1 can modify proj_1
// `p, proj_member_1, proj_1, modify` : role proj_member_1 can modify proj_1

// default roles
//
// `g, 0x183F09C3cE99C02118c570e03808476b22d63191, hall`
