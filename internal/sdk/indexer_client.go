package sdk

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"
)

type SeedHolderRecord struct {
	Wallet string   `json:"wallet"`
	Ids    []string `json:"ids"`
}

type IndexerClient struct {
	ApiBase string `json:"api_base"`
}

var indexerClient *IndexerClient

func InitIndexerClient(apiBase string) error {
	indexerClient = &IndexerClient{
		ApiBase: apiBase,
	}
	return nil
}

func GetIndexerClient() *IndexerClient {
	return indexerClient
}

func (c *IndexerClient) GetSeedHolderInfo(endTimestamp int64) ([]*SeedHolderRecord, error) {
	endpoint := fmt.Sprintf("%s/erc721/snapshot/0x30093266E34a816a53e302bE3e59a93B52792FD4/%d", c.ApiBase, endTimestamp)
	log.Debug().Msgf("Try to get seed holder data, endpoint is %s", endpoint)

	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got error response from indexer endpoint: status code: %d, resp: %+v", resp.StatusCode, resp)
	}

	var respData []*SeedHolderRecord
	err = json.NewDecoder(resp.Body).Decode(&respData)
	if err != nil {
		return nil, err
	}

	return respData, nil
}
