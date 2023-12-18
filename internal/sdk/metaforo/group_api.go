package metaforo

import (
	"encoding/json"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/common"
)

func GetGroupInfo(groupName string) {
	apiPath := "/api/group/info"
	statusCode, respObject, err := common.DoHttpRequest[GroupInfoResponse](&common.HttpRequestData{
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
