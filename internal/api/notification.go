package api

import (
	"fmt"
	"strconv"
)

// English: "en", Chinese: "zh"
const (
	LanguageZH = "zh"
	LanguageEN = "en"
)

const (
	NotificationTypeProjStaffAdd     = "proj_staff_add"
	NotificationTypeProjStaffRemove  = "proj_staff_remove"
	NotificationTypeGuildStaffAdd    = "guild_staff_add"
	NotificationTypeGuildStaffRemove = "guild_staff_remove"

	NotificationTypeObtainAssert = "obtain_assert"
)

// GenerateProjectStaffAddNotificationParams generate params for project's staff added.
/*
{
 "type": "proj_staff_add",
 "data": {
  	"proj_id": 1
 }
}
*/
func GenerateProjectStaffAddNotificationParams(projectID uint, projectName string) (title map[string]string, body map[string]string, payload map[string]string) {
	title[LanguageEN] = "Project notification"
	title[LanguageZH] = "项目提示"

	body[LanguageEN] = fmt.Sprintf("You have been added to %s project", projectName)
	body[LanguageZH] = fmt.Sprintf("你已被添加为 %s 项目的成员", projectName)

	payload = map[string]string{
		"type":    NotificationTypeProjStaffAdd,
		"proj_id": strconv.Itoa(int(projectID)),
	}

	return
}

// GenerateProjectStaffRemoveNotificationParams generate params for project's staff removed.
func GenerateProjectStaffRemoveNotificationParams(projectID uint, projectName string) (title map[string]string, body map[string]string, payload map[string]string) {
	title[LanguageEN] = "Project notification"
	title[LanguageZH] = "项目提示"

	body[LanguageEN] = fmt.Sprintf("You have been removed by %s project", projectName)
	body[LanguageZH] = fmt.Sprintf("你已被 %s 项目移除", projectName)

	payload = map[string]string{
		"type":    NotificationTypeProjStaffRemove,
		"proj_id": strconv.Itoa(int(projectID)),
	}

	return
}

// GenerateGuildStaffAddNotificationParams generate params for guild's staff added.
/*
{
 "type": "guild_staff_add",
 "data": {
  	"guild_id": 1
 }
}
*/
func GenerateGuildStaffAddNotificationParams(guildID uint, guildName string) (title map[string]string, body map[string]string, payload map[string]string) {
	title[LanguageEN] = "Guild notification"
	title[LanguageZH] = "公会提示"

	body[LanguageEN] = fmt.Sprintf("You have been added to %s guild", guildName)
	body[LanguageZH] = fmt.Sprintf("你已被添加为 %s 公会的成员", guildName)

	payload = map[string]string{
		"type":     NotificationTypeGuildStaffAdd,
		"guild_id": strconv.Itoa(int(guildID)),
	}

	return
}

// GenerateGuildStaffRemoveNotificationParams generate params for guild's staff removed.
func GenerateGuildStaffRemoveNotificationParams(guildID uint, guildName string) (title map[string]string, body map[string]string, payload map[string]string) {
	title[LanguageEN] = "Guild notification"
	title[LanguageZH] = "公会提示"

	body[LanguageEN] = fmt.Sprintf("You have been removed by %s guild", guildName)
	body[LanguageZH] = fmt.Sprintf("你已被 %s 公会移除", guildName)

	payload = map[string]string{
		"type":     NotificationTypeGuildStaffRemove,
		"guild_id": strconv.Itoa(int(guildID)),
	}

	return
}

// GenerateObtainAssertNotificationParams generate params for user obtained assert.
/*
{
 "type": "receive_assert",
 "data": {
  	"name": "Points",
	"amount": 120
 }
}
*/
func GenerateObtainAssertNotificationParams(assertName string, amount string) (title map[string]string, body map[string]string, payload map[string]string) {
	title[LanguageEN] = "Personal assets"
	title[LanguageZH] = "个人资产"

	body[LanguageEN] = fmt.Sprintf("%s %s received", amount, assertName)
	body[LanguageZH] = fmt.Sprintf("已收到 %s %s", amount, assertName)

	payload = map[string]string{
		"type":          NotificationTypeObtainAssert,
		"assert_name":   assertName,
		"assert_amount": amount,
	}

	return
}
