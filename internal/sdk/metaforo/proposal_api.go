package metaforo

import (
	"encoding/json"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/common"
)

func GetProposals(accessToken string, paginationParams *PaginationParams) {
	apiPath := "/api/thread/list"
	statusCode, respObject, err := common.DoHttpRequest[ProposalsResponse](&common.HttpRequestData{
		ApiUri:      apiBase + apiPath,
		HttpMethod:  "GET",
		QueryParams: paginationParams.ToMap(),
		Header:      BaseHeader,
	})
	if err != nil {
		log.Error().Msgf("Send request error: %s", err)
	}

	respBytes, err := json.MarshalIndent(respObject, "", "  ")
	if err != nil {
		log.Error().Msgf("Marshal error: %s", err)
	}

	log.Debug().Msgf("Resp status code: %d, Content:", statusCode)
	os.Stdout.Write(respBytes)
}
