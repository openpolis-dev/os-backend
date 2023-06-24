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
	title.SetEn("Join Project")
	title.SetZhHans("加入项目")

	body.SetEn(fmt.Sprintf("You ard added to Project %s", projectName))
	body.SetZhHans(fmt.Sprintf("You ard added to Project %s", projectName))

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
	title.SetEn("Quit Project")
	title.SetZhHans("退出项目")

	body.SetEn(fmt.Sprintf("You ard removed from Project %s", projectName))
	body.SetZhHans(fmt.Sprintf("You ard removed from Project %s", projectName))

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
	title.SetEn("Join Guild")
	title.SetZhHans("加入工会")

	body.SetEn(fmt.Sprintf("You ard added to Guild %s", guildName))
	body.SetZhHans(fmt.Sprintf("You ard added to Guild %s", guildName))

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
	title.SetEn("Quit Guild")
	title.SetZhHans("退出工会")

	body.SetEn(fmt.Sprintf("You ard removed from Guild %s", guildName))
	body.SetZhHans(fmt.Sprintf("You ard removed from Guild %s", guildName))

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
	title.SetEn("Assert Obtained")
	title.SetZhHans("Assert Obtained")

	body.SetEn(fmt.Sprintf("You have obtained %d %s", amount, assertName))
	body.SetZhHans(fmt.Sprintf("You have obtained %d %s", amount, assertName))

	data = map[string]any{
		"type": NotificationTypeObtainAssert,
		"data": map[string]any{
			"name":   assertName,
			"amount": amount,
		},
	}

	return
}
