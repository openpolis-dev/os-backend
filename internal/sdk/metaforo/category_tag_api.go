package metaforo

import (
	"encoding/json"
	"maps"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/common"
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

func GetTags(groupName string, accessToken string) {
	apiPath := "/api/label/get"

	formHeader := make(map[string]string)
	maps.Copy(formHeader, BaseHeader)
	formHeader["Content-Type"] = "application/x-www-form-urlencoded"
	formHeader["Authorization"] = "Bearer " + accessToken

	requestFormBody := []byte(`group_name=` + groupName)

	statusCode, respObject, err := common.DoHttpRequest[TagsListResponse](&common.HttpRequestData{
		ApiUri:     apiBase + apiPath,
		HttpMethod: "POST",
		Header:     formHeader,
		BodyBytes:  requestFormBody,
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
