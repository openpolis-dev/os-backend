package api

const (
	NotificationTypeProjStaffAdd     = "proj_staff_add"
	NotificationTypeProjStaffRemove  = "proj_staff_remove"
	NotificationTypeGuildStaffAdd    = "guild_staff_add"
	NotificationTypeGuildStaffRemove = "guild_staff_remove"

	NotificationTypeReceiveAssert = "receive_assert"
)

// GenerateProjectStaffAddData generate data for project's staff added.
/*
{
 "type": "proj_staff_add",
 "data": {
  	"proj_id": 1
 }
}
*/
func GenerateProjectStaffAddData(projectID uint) map[string]any {
	return map[string]any{
		"type": NotificationTypeProjStaffAdd,
		"data": map[string]any{
			"proj_id": projectID,
		},
	}
}

// GenerateProjectStaffRemoveData generate data for project's staff removed.
func GenerateProjectStaffRemoveData(projectID uint) map[string]any {
	return map[string]any{
		"type": NotificationTypeProjStaffRemove,
		"data": map[string]any{
			"proj_id": projectID,
		},
	}
}

// GenerateReceiveAssertData generate data for user receive assert
/*
{
 "type": "receive_assert",
 "data": {
  	"name": "Points",
	"amount": 120
 }
}
*/
func GenerateReceiveAssertData(assertName string, amount int64) map[string]any {
	return map[string]any{
		"type": NotificationTypeReceiveAssert,
		"data": map[string]any{
			"name":   assertName,
			"amount": amount,
		},
	}
}
