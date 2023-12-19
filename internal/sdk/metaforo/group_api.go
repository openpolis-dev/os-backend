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
