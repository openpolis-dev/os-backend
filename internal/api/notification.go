package api

import "fmt"

const (
	NotificationTypeProjStaffAdd     = "proj_staff_add"
	NotificationTypeProjStaffRemove  = "proj_staff_remove"
	NotificationTypeGuildStaffAdd    = "guild_staff_add"
	NotificationTypeGuildStaffRemove = "guild_staff_remove"

	NotificationTypeReceiveAssert = "receive_assert"
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
func GenerateProjectStaffAddNotificationParams(projectID uint, projectName string) (title string, body string, data map[string]any) {
	title = "Join Project"
	body = fmt.Sprintf("You ard added to Project %s", projectName)
	data = map[string]any{
		"type": NotificationTypeProjStaffAdd,
		"data": map[string]any{
			"proj_id": projectID,
		},
	}

	return
}

// GenerateProjectStaffRemoveNotificationParams generate params for project's staff removed.
func GenerateProjectStaffRemoveNotificationParams(projectID uint, projectName string) (title string, body string, data map[string]any) {
	title = "Quit Project"
	body = fmt.Sprintf("You ard removed from Project %s", projectName)
	data = map[string]any{
		"type": NotificationTypeProjStaffRemove,
		"data": map[string]any{
			"proj_id": projectID,
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
func GenerateObtainAssertNotificationParams(assertName string, amount int64) (title string, body string, data map[string]any) {
	title = "Assert Obtained"
	body = fmt.Sprintf("You have obtained %d %s", amount, assertName)
	data = map[string]any{
		"type": NotificationTypeReceiveAssert,
		"data": map[string]any{
			"name":   assertName,
			"amount": amount,
		},
	}

	return
}
