package metaforo

import "fmt"

// UserActivities get user activities
// `filter` parameter values: all post likes tips
// only `perPage` pageable parameter, not support page parameter
func UserActivities(accessToken, userId, filter string, perPage string) ([]*UserActivity, error) {
	apiPath := fmt.Sprintf("/api/profile/%s/activities", userId)

	// prepare headers
	formHeader := AuthHeader(accessToken)

	// send request
	_, resp, err := doHttpRequest[UserActivitiesResponse](&httpRequestData{
		ApiUri:     apiBase + apiPath,
		HttpMethod: "GET",
		QueryParams: map[string]string{
			"filter":   filter,
			"per_page": perPage,
		},
		Header: formHeader,
	})
	if err != nil {
		return nil, err
	}

	return resp.UserActivities, nil
}

func UserDetail(userId string) (*UserDetailResponse, error) {
	apiPath := fmt.Sprintf("/api/profile/%s", userId)

	// send request
	_, resp, err := doHttpRequest[UserDetailResponse](&httpRequestData{
		ApiUri:     apiBase + apiPath,
		HttpMethod: "GET",
		Header:     BaseHeader,
	})
	if err != nil {
		return nil, err
	}

	return resp, nil
}
