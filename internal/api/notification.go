package api

import (
	"fmt"

	"github.com/OneSignal/onesignal-go-api"
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
func GenerateProjectStaffAddNotificationParams(projectID uint, projectName string) (title *onesignal.StringMap, body *onesignal.StringMap, data map[string]any) {
	title = &onesignal.StringMap{}
	title.SetEn("Project notification")
	title.SetZhHans("项目提示")

	body = &onesignal.StringMap{}
	body.SetEn(fmt.Sprintf("You have been added to %s project", projectName))
	body.SetZhHans(fmt.Sprintf("你已被添加为 %s 项目的成员", projectName))

	data = map[string]any{
		"type": NotificationTypeProjStaffAdd,
		"data": map[string]any{
			"proj_id": projectID,
		},
	}

	return
}

// GenerateProjectStaffRemoveNotificationParams generate params for project's staff removed.
func GenerateProjectStaffRemoveNotificationParams(projectID uint, projectName string) (title *onesignal.StringMap, body *onesignal.StringMap, data map[string]any) {
	title = &onesignal.StringMap{}
	title.SetEn("Project notification")
	title.SetZhHans("项目提示")

	body = &onesignal.StringMap{}
	body.SetEn(fmt.Sprintf("x %s", projectName))
	body.SetZhHans(fmt.Sprintf("x %s", projectName))

	data = map[string]any{
		"type": NotificationTypeProjStaffRemove,
		"data": map[string]any{
			"proj_id": projectID,
		},
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
func GenerateGuildStaffAddNotificationParams(guildID uint, guildName string) (title *onesignal.StringMap, body *onesignal.StringMap, data map[string]any) {
	title = &onesignal.StringMap{}
	title.SetEn("Guild notification")
	title.SetZhHans("公会提示")

	body = &onesignal.StringMap{}
	body.SetEn(fmt.Sprintf("You have been added to %s guild", guildName))
	body.SetZhHans(fmt.Sprintf("你已被添加为 %s 公会的成员", guildName))

	data = map[string]any{
		"type": NotificationTypeGuildStaffAdd,
		"data": map[string]any{
			"guild_id": guildID,
		},
	}

	return
}

// GenerateGuildStaffRemoveNotificationParams generate params for guild's staff removed.
func GenerateGuildStaffRemoveNotificationParams(guildID uint, guildName string) (title *onesignal.StringMap, body *onesignal.StringMap, data map[string]any) {
	title = &onesignal.StringMap{}
	title.SetEn("Guild notification")
	title.SetZhHans("公会提示")

	body = &onesignal.StringMap{}
	body.SetEn(fmt.Sprintf("x %s", guildName))
	body.SetZhHans(fmt.Sprintf("x %s", guildName))

	data = map[string]any{
		"type": NotificationTypeGuildStaffRemove,
		"data": map[string]any{
			"guild_id": guildID,
		},
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
func GenerateObtainAssertNotificationParams(assertName string, amount int64) (title *onesignal.StringMap, body *onesignal.StringMap, data map[string]any) {
	title.SetEn("Personal assets")
	title.SetZhHans("个人资产")

	body.SetEn(fmt.Sprintf("%d %s received", amount, assertName))
	body.SetZhHans(fmt.Sprintf("已收到 %d %s", amount, assertName))

	data = map[string]any{
		"type": NotificationTypeObtainAssert,
		"data": map[string]any{
			"name":   assertName,
			"amount": amount,
		},
	}

	return
}
