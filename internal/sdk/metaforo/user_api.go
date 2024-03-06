package metaforo

import "fmt"

// UserActivities get user activities
// `filter` parameter values: all post likes tips
// only `perPage` pageable parameter, not support page parameter
func UserActivities(userId, filter, perPage, session string) ([]*UserActivity, string, error) {
	apiPath := fmt.Sprintf("/api/profile/%s/activities", userId)

	// send request
	_, resp, err := doHttpRequest[UserActivitiesResponse](&httpRequestData{
		ApiUri:     apiBase + apiPath,
		HttpMethod: "GET",
		QueryParams: map[string]string{
			"filter":   filter,
			"per_page": perPage,
			"session":  session,
		},
		Header: BaseHeader,
	})
	if err != nil {
		return nil, "", err
	}

	return resp.UserActivities, resp.Session, nil
}

func UserDetail(userId int) (*UserDetailResponseForProfileAPI, error) {
	apiPath := fmt.Sprintf("/api/profile/%d", userId)

	// send request
	_, resp, err := doHttpRequest[UserDetailResponseForProfileAPI](&httpRequestData{
		ApiUri:     apiBase + apiPath,
		HttpMethod: "GET",
		Header:     BaseHeader,
	})
	if err != nil {
		return nil, err
	}

	return resp, nil
}
