package model

import (
	"fmt"

	"github.com/casbin/casbin/v2"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/common"
)

func generateCasbinPoliciesForEntity(entityId uint, entityName string) [][]string {
	var sponsorPrefix string
	var objPrefix string
	switch entityName {
	case "project":
		sponsorPrefix = internal.RoleProjSponsorPrefix
		objPrefix = internal.ObjProjPrefix
	case "guild":
		sponsorPrefix = internal.RoleGuildSponsorPrefix
		objPrefix = internal.ObjGuildPrefix
	default:
		log.Error().Msgf("unknown entity name: %s", entityName)
		return nil
	}

	return [][]string{
		// Policies for entity sponsor
		// p, <entity>_sponsor_1, <entity>_1, modify
		// p, <entity>_sponsor_1, <entity>_1, create_app
		// p, <entity>_sponsor_1, <entity>_1, u_member
		// p, <entity>_sponsor_1, <entity>_1, u_budget
		{fmt.Sprintf("%s%d", sponsorPrefix, entityId), fmt.Sprintf("%s%d", objPrefix, entityId), internal.ActModify},
		{fmt.Sprintf("%s%d", sponsorPrefix, entityId), fmt.Sprintf("%s%d", objPrefix, entityId), internal.ActCreateApplication},
		{fmt.Sprintf("%s%d", sponsorPrefix, entityId), fmt.Sprintf("%s%d", objPrefix, entityId), internal.ActUpdateMember},
		{fmt.Sprintf("%s%d", sponsorPrefix, entityId), fmt.Sprintf("%s%d", objPrefix, entityId), internal.ActUpdateBudget},

		// Policies for entity members is not using for now, which should have the format as below:
		//// p, <entity>_member_1, <entity>_1, modify
		//// p, <entity>_member_1, <entity>_1, create_app
		//{fmt.Sprintf("%s%d", memberPrefix, entityId), fmt.Sprintf("%s%d", objPrefix, entityId), api.ActModify},
		//{fmt.Sprintf("%s%d", memberPrefix, entityId), fmt.Sprintf("%s%d", objPrefix, entityId), api.ActCreateApplication},
	}
}

func generateGroupingPoliciesForEntity(entityId uint, sponsorList []string, entityName string) [][]string {
	roleSponsorPrefix := ""
	switch entityName {
	case "project":
		roleSponsorPrefix = internal.RoleProjSponsorPrefix
	case "guild":
		roleSponsorPrefix = internal.RoleGuildSponsorPrefix
	default:
		log.Error().Msgf("unknown entity name: %s", entityName)
		return nil
	}

	sponsorGroupingPolicies := lo.Map(sponsorList, func(sponsorWallet string, _ int) []string {
		// g, 0xc13..1283 <entity>_sponsor_1
		return []string{common.FormatUserWallet(sponsorWallet), fmt.Sprintf("%s%d", roleSponsorPrefix, entityId)}
	})
	return sponsorGroupingPolicies

	// Member grouping policies, not using for now. The code below should be updated for support entities if be activated.
	//memberGroupingPolicies := lo.Map(memberList, func(memberWallet string, _ int) []string {
	//	// g, 0xc13..1283 proj_member_1
	//	return []string{strings.ToLower(memberWallet), fmt.Sprintf("%s%d", internal.RoleProjMemberPrefix, projectId)}
	//})
	//groupingPolicies := append(memberGroupingPolicies, sponsorGroupingPolicies...)
	//return groupingPolicies
}

// GenerateCasbinPoliciesForProject generate casbin policies for specified project
func GenerateCasbinPoliciesForProject(projectId uint) [][]string {
	return generateCasbinPoliciesForEntity(projectId, "project")
}

func GenerateCasbinPoliciesForGuild(guildId uint) [][]string {
	return generateCasbinPoliciesForEntity(guildId, "guild")
}

func GenerateGroupingPoliciesForProject(projectId uint, sponsorsList []string, memberList []string) [][]string {
	return generateGroupingPoliciesForEntity(projectId, sponsorsList, "project")
}
func GenerateGroupingPoliciesForGuild(guildId uint, sponsorList []string) [][]string {
	return generateGroupingPoliciesForEntity(guildId, sponsorList, "guild")
}

func SetWalletPermissionAsProjectSponsor(enforcer *casbin.SyncedEnforcer, projectId uint, walletList []string) error {
	policies := GenerateCasbinPoliciesForProject(projectId)
	log.Debug().Msgf("policies for project %d: %+v", projectId, policies)
	_, err = enforcer.AddPolicies(policies)
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

func SetWalletPermissionAsGuildSponsor(enforcer *casbin.SyncedEnforcer, guildId uint, walletList []string) error {
	// add policies
	policies := GenerateGroupingPoliciesForGuild(guildId, walletList)
	log.Debug().Msgf("policies for guild %d: %+v", guildId, policies)
	_, err = enforcer.AddPolicies(policies)
	if err != nil {
		log.Error().Msgf("create guild policies error: %+v", err)
		return err
	}

	// add roles
	sponsorGroupingPolicies := GenerateGroupingPoliciesForGuild(guildId, walletList)
	_, err = enforcer.AddGroupingPolicies(sponsorGroupingPolicies)
	if err != nil {
		log.Error().Msgf("add grouping policies error: %+v", err)
		return err
	}
	return enforcer.SavePolicy()
}

func RemoveWalletPermissionFromGuildSponsor(enforcer *casbin.SyncedEnforcer, guildId uint, walletList []string) error {
	groupingPolicies := GenerateGroupingPoliciesForGuild(guildId, walletList)
	_, err = enforcer.RemoveGroupingPolicies(groupingPolicies)
	if err != nil {
		log.Error().Msgf("Remove grouping policies error: %+v", err)
		return err
	}
	return enforcer.SavePolicy()
}
