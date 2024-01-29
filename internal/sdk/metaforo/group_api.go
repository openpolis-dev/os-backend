package metaforo

func GetGroupInfo(groupName string) (*GroupInfo, error) {
	apiPath := "/api/group/info"

	// send request
	_, resp, err := doHttpRequest[GroupInfoResponse](&httpRequestData{
		ApiUri:     apiBase + apiPath,
		HttpMethod: "GET",
		QueryParams: map[string]string{
			"group_name": groupName,
		},
		Header: BaseHeader,
	})
	if err != nil {
		return nil, err
	}

	return resp.Group, nil
}

func JoinGroup(accessToken, groupName string) error {
	apiPath := "/api/follow"

	// send request
	_, _, err := doHttpRequest[struct{}](&httpRequestData{
		ApiUri:     apiBase + apiPath,
		HttpMethod: "POST",
		QueryParams: map[string]string{
			"group_name": groupName,
		},
		Header: AuthHeader(accessToken),
	})
	if err != nil {
		return err
	}

	return nil
}

func LeaveGroup(accessToken, groupName string) error {
	apiPath := "/api/unfollow"

	// send request
	_, _, err := doHttpRequest[struct{}](&httpRequestData{
		ApiUri:     apiBase + apiPath,
		HttpMethod: "POST",
		QueryParams: map[string]string{
			"group_name": groupName,
		},
		Header: AuthHeader(accessToken),
	})
	if err != nil {
		return err
	}

	return nil
}
