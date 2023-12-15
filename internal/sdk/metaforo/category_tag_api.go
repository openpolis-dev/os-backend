package metaforo

import (
	"encoding/json"
	"maps"
	"os"
	"strconv"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/valyala/fasthttp"
)

func GetCategories(groupName string) {
	apiPath := "/api/get_categories"
	statusCode, respObject, err := common.DoHttpRequest[CategoriesListResponse](&common.HttpRequestData{
		ApiUri:     apiBase + apiPath,
		HttpMethod: "GET",
		QueryParams: map[string]string{
			"group_name": groupName,
		},
		Header: BaseHeader,
	})
	if err != nil {
		log.Error().Msgf("Send request error: %s", err)
		return
	}

	respBytes, err := json.MarshalIndent(respObject, "", "  ")
	if err != nil {
		log.Error().Msgf("Marshal error: %s", err)
		return
	}

	log.Debug().Msgf("Resp status code: %d, Content:", statusCode)
	os.Stdout.Write(respBytes)
}

func NewCategory(groupName string, categoryName string, parentId int, accessToken string) {
	apiPath := "/api/category/add"

	formHeader := make(map[string]string)
	maps.Copy(formHeader, BaseHeader)
	formHeader["Content-Type"] = "application/x-www-form-urlencoded"
	formHeader["Authorization"] = "Bearer " + accessToken

	requestParams := fasthttp.Args{}
	requestParams.Add("group_name", groupName)
	requestParams.Add("name", categoryName)
	// [{"id":0,"name":"Everyone","can_see":1,"can_reply":1,"can_create":1}]
	// TODO: Add permission list
	//requestParams.Add("permission_list", categoryName)
	if parentId != 0 {
		requestParams.Add("parent_id", strconv.Itoa(parentId))
	}

	header := getAuthHeader(accessToken)

	statusCode, respObject, err := common.DoHttpRequest[TagsListResponse](&common.HttpRequestData{
		ApiUri:         apiBase + apiPath,
		HttpMethod:     "POST",
		Header:         header,
		FormBodyParams: &requestParams,
	})
	if err != nil {
		log.Error().Msgf("Send request error: %s", err)
		return
	}

	respBytes, err := json.MarshalIndent(respObject, "", "  ")
	if err != nil {
		log.Error().Msgf("Marshal error: %s", err)
		return
	}

	log.Debug().Msgf("Resp status code: %d, Content:", statusCode)
	os.Stdout.Write(respBytes)
}

func GetTags(groupName string, accessToken string) {
	apiPath := "/api/label/get"

	header := getAuthHeader(accessToken)

	requestParams := fasthttp.Args{}
	requestParams.Add("group_name", groupName)

	statusCode, respObject, err := common.DoHttpRequest[TagsListResponse](&common.HttpRequestData{
		ApiUri:         apiBase + apiPath,
		HttpMethod:     "POST",
		Header:         header,
		FormBodyParams: &requestParams,
	})
	if err != nil {
		log.Error().Msgf("Send request error: %s", err)
		return
	}

	respBytes, err := json.MarshalIndent(respObject, "", "  ")
	if err != nil {
		log.Error().Msgf("Marshal error: %s", err)
		return
	}

	log.Debug().Msgf("Resp status code: %d, Content:", statusCode)
	os.Stdout.Write(respBytes)
}

func getAuthHeader(accessToken string) map[string]string {
	header := make(map[string]string)
	maps.Copy(header, BaseHeader)
	header["Authorization"] = "Bearer " + accessToken
	return header

}
