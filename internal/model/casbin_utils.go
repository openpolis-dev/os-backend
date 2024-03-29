package model

import (
	"fmt"
	"strings"

	"github.com/casbin/casbin/v2"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/common"
)

// GenerateCasbinPoliciesForProject generate casbin policies for specified project
func GenerateCasbinPoliciesForProject(projectId uint) [][]string {
	return [][]string{
		// Policies for project sponsor
		// p, proj_sponsor_1, proj_1, modify
		// p, proj_sponsor_1, proj_1, create_app
		// p, proj_sponsor_1, proj_1, u_member
		// p, proj_sponsor_1, proj_1, u_budget
		{fmt.Sprintf("%s%d", internal.RoleProjSponsorPrefix, projectId), fmt.Sprintf("%s%d", internal.ObjProjPrefix, projectId), internal.ActModify},
		{fmt.Sprintf("%s%d", internal.RoleProjSponsorPrefix, projectId), fmt.Sprintf("%s%d", internal.ObjProjPrefix, projectId), internal.ActCreateApplication},
		{fmt.Sprintf("%s%d", internal.RoleProjSponsorPrefix, projectId), fmt.Sprintf("%s%d", internal.ObjProjPrefix, projectId), internal.ActUpdateMember},
		{fmt.Sprintf("%s%d", internal.RoleProjSponsorPrefix, projectId), fmt.Sprintf("%s%d", internal.ObjProjPrefix, projectId), internal.ActUpdateBudget},

		// Policies for project members (is not using for now)
		//// p, proj_member_1, proj_1, modify
		//// p, proj_member_1, proj_1, create_app
		//{fmt.Sprintf("%s%d", api.RoleProjMemberPrefix, proj.ID), fmt.Sprintf("%s%d", api.ObjProjPrefix, proj.ID), api.ActModify},
		//{fmt.Sprintf("%s%d", api.RoleProjMemberPrefix, proj.ID), fmt.Sprintf("%s%d", api.ObjProjPrefix, proj.ID), api.ActCreateApplication},
	}
}

func GenerateGroupingPoliciesForProject(projectId uint, sponsorsList []string, memberList []string) [][]string {
	sponsorGroupingPolicies := lo.Map(sponsorsList, func(sponsorWallet string, _ int) []string {
		// g, 0xc13..1283 proj_sponsor_1
		return []string{common.FormatUserWallet(sponsorWallet), fmt.Sprintf("%s%d", internal.RoleProjSponsorPrefix, projectId)}
	})
	return sponsorGroupingPolicies

	//memberGroupingPolicies := lo.Map(memberList, func(memberWallet string, _ int) []string {
	//	// g, 0xc13..1283 proj_member_1
	//	return []string{strings.ToLower(memberWallet), fmt.Sprintf("%s%d", internal.RoleProjMemberPrefix, projectId)}
	//})
	//groupingPolicies := append(memberGroupingPolicies, sponsorGroupingPolicies...)
	//return groupingPolicies
}

func CreateProjectCasbinPolicies(enforcer *casbin.SyncedEnforcer, projectId uint) error {
	policies := GenerateCasbinPoliciesForProject(projectId)
	log.Debug().Msgf("policies for project %d: %+v", projectId, policies)
	_, err = enforcer.AddPolicies(policies)
	return err
}

func SetWalletPermissionAsProjectSponsor(enforcer *casbin.SyncedEnforcer, projectId uint, walletList []string) error {
	err = CreateProjectCasbinPolicies(enforcer, projectId)
	if err != nil {
		log.Error().Msgf("Create project policies error: %+v", err)
		return err
	}

	groupingPolicies := GenerateGroupingPoliciesForProject(projectId, walletList, nil)
	log.Debug().Msgf("grouping policies for project %d: %+v", projectId, groupingPolicies)
	if _, err = enforcer.AddGroupingPolicies(groupingPolicies); err != nil {
		log.Error().Msgf("Add grouping policies error: %+v", err)
		return err
	}

	return enforcer.SavePolicy()
}

func RemoveWalletPermissionFromProjectSponsor(enforcer *casbin.SyncedEnforcer, projectId uint, walletList []string) error {
	groupingPolicies := GenerateGroupingPoliciesForProject(projectId, walletList, nil)
	_, err = enforcer.RemoveGroupingPolicies(groupingPolicies)
	if err != nil {
		log.Error().Msgf("Remove grouping policies error: %+v", err)
		return err
	}
	return enforcer.SavePolicy()
}

func GenerateGroupingPoliciesForGuild(guildId uint, memberList []string) [][]string {
	return lo.Map(memberList, func(memberWallet string, _ int) []string {
		// g, 0xc13..1283 guild_member_1
		return []string{strings.ToLower(memberWallet), fmt.Sprintf("%s%d", internal.RoleGuildSponsorPrefix, guildId)}
	})
	//memberGroupingPolicies := lo.Map(req.Members, func(member string, _ int) []string {
	//	// g, 0xc13..1283 guild_member_1
	//	return []string{strings.ToLower(member), fmt.Sprintf("%s%d", api.RoleGuildMemberPrefix, guild.ID)}
	//})
	//groupingPolicies := append(memberGroupingPolicies, sponsorGroupingPolicies...)
}

func SetWalletPermissionAsGuildSponsor(enforcer *casbin.SyncedEnforcer, guildId uint, wallet string) error {
	// add policies
	policies := [][]string{
		// p, guild_sponsor_1, guild_1, modify
		// p, guild_sponsor_1, guild_1, create_app
		// p, guild_sponsor_1, guild_1, u_member
		// p, guild_sponsor_1, guild_1, u_budget
		{fmt.Sprintf("%s%d", internal.RoleGuildSponsorPrefix, guildId), fmt.Sprintf("%s%d", internal.ObjGuildPrefix, guildId), internal.ActModify},
		{fmt.Sprintf("%s%d", internal.RoleGuildSponsorPrefix, guildId), fmt.Sprintf("%s%d", internal.ObjGuildPrefix, guildId), internal.ActCreateApplication},
		{fmt.Sprintf("%s%d", internal.RoleGuildSponsorPrefix, guildId), fmt.Sprintf("%s%d", internal.ObjGuildPrefix, guildId), internal.ActUpdateMember},
		{fmt.Sprintf("%s%d", internal.RoleGuildSponsorPrefix, guildId), fmt.Sprintf("%s%d", internal.ObjGuildPrefix, guildId), internal.ActUpdateBudget},
		//// p, guild_member_1, guild_1, modify
		//// p, guild_member_1, guild_1, create_app
		//{fmt.Sprintf("%s%d", api.RoleGuildMemberPrefix, guild.ID), fmt.Sprintf("%s%d", api.ObjGuildPrefix, guild.ID), api.ActModify},
		//{fmt.Sprintf("%s%d", api.RoleGuildMemberPrefix, guild.ID), fmt.Sprintf("%s%d", api.ObjGuildPrefix, guild.ID), api.ActCreateApplication},
	}
	_, err = enforcer.AddPolicies(policies)
	if err != nil {
		log.Error().Msgf("create guild policies error: %+v", err)
		return err
	}

	// add roles
	sponsorGroupingPolicies := GenerateGroupingPoliciesForGuild(guildId, []string{wallet})
	_, err = enforcer.AddGroupingPolicies(sponsorGroupingPolicies)
	if err != nil {
		log.Error().Msgf("add grouping policies error: %+v", err)
		return err
	}
	return enforcer.SavePolicy()
}
func RemoveWalletPermissionAsGuildSponsor(enforcer *casbin.SyncedEnforcer, guildId uint, wallet string) error {
	groupingPolicies := GenerateGroupingPoliciesForGuild(guildId, []string{wallet})
	_, err = enforcer.RemoveGroupingPolicies(groupingPolicies)
	if err != nil {
		log.Error().Msgf("Remove grouping policies error: %+v", err)
		return err
	}
	return enforcer.SavePolicy()
}
