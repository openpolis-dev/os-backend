package metaforo

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"

	"github.com/rs/zerolog/log"
)

func GetCategories(groupName string) ([]*Category, error) {
	apiPath := "/api/get_categories"
	_, resp, err := doHttpRequest[CategoriesListResponse](&httpRequestData{
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

	return resp.Categories, nil
}

func NewCategory(accessToken, groupName string, categoryName, iconUnicode, parentId, templateId string, permissionList []*NewCategoryPermissionRequest) error {
	apiPath := "/api/category/add"

	// prepare headers
	formHeader := AuthHeader(accessToken)

	// prepare multipart body
	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	_ = writer.WriteField("name", categoryName)
	_ = writer.WriteField("icon_unicode", iconUnicode)
	_ = writer.WriteField("parent_id", parentId)
	_ = writer.WriteField("template_id", templateId)
	_ = writer.WriteField("group_name", groupName)
	if permissionList != nil {
		p, _ := json.Marshal(permissionList)
		_ = writer.WriteField("permission_list", string(p))
	}
	err := writer.Close()
	if err != nil {
		log.Error().Msgf("Prepare Multipart paramter error: %s", err)
		return err
	}

	// send request
	_, _, err = doHttpRequest[struct{}](&httpRequestData{
		ApiUri:               apiBase + apiPath,
		HttpMethod:           http.MethodPost,
		MultipartBodyParams:  payload.Bytes(),
		MultipartContentType: writer.FormDataContentType(),
		Header:               formHeader,
	})

	return err
}

func UpdateCategory(accessToken, groupName string, categoryId, categoryName, iconUnicode, parentId, templateId string, permissionList []*NewCategoryPermissionRequest) error {
	apiPath := "/api/category/update"

	// prepare headers
	formHeader := AuthHeader(accessToken)

	// prepare multipart body
	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	_ = writer.WriteField("id", categoryId)
	_ = writer.WriteField("name", categoryName)
	_ = writer.WriteField("icon_unicode", iconUnicode)
	_ = writer.WriteField("parent_id", parentId)
	_ = writer.WriteField("template_id", templateId)
	_ = writer.WriteField("group_name", groupName)
	if permissionList != nil {
		p, _ := json.Marshal(permissionList)
		_ = writer.WriteField("permission_list", string(p))
	}
	err := writer.Close()
	if err != nil {
		log.Error().Msgf("Prepare Multipart paramter error: %s", err)
		return err
	}

	// send request
	_, _, err = doHttpRequest[struct{}](&httpRequestData{
		ApiUri:               apiBase + apiPath,
		HttpMethod:           http.MethodPost,
		MultipartBodyParams:  payload.Bytes(),
		MultipartContentType: writer.FormDataContentType(),
		Header:               formHeader,
	})

	return err
}

func DeleteCategory(accessToken, groupName, categoryId string) error {
	apiPath := "/api/category/delete"

	// prepare headers
	formHeader := AuthHeader(accessToken)

	// prepare multipart body
	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	_ = writer.WriteField("id", categoryId)
	_ = writer.WriteField("group_name", groupName)
	err := writer.Close()
	if err != nil {
		log.Error().Msgf("Prepare Multipart paramter error: %s", err)
		return err
	}

	// send request
	_, _, err = doHttpRequest[struct{}](&httpRequestData{
		ApiUri:               apiBase + apiPath,
		HttpMethod:           http.MethodPost,
		MultipartBodyParams:  payload.Bytes(),
		MultipartContentType: writer.FormDataContentType(),
		Header:               formHeader,
	})

	return err
}
