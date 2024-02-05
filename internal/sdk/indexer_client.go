package sdk

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
)

type SeedHolderRecord struct {
	Wallet string   `json:"wallet"`
	Ids    []string `json:"ids"`
}

type SeasonSBTRecord struct {
	Wallet string   `json:"wallet"`
	Ids    []string `json:"ids"`
	Values []string `json:"values"`
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
	endpoint := fmt.Sprintf("%s/snapshot/%s/%s/%d", c.ApiBase, internal.SeedContractType, internal.SeedContractAddr, endTimestamp)
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

func (c *IndexerClient) GetCurrentSeedHolderCount() int {
	seedHolderRecords, err := c.GetSeedHolderInfo(time.Now().UTC().Unix())
	if err != nil {
		log.Error().Msgf("get seed holder info error: %+v", err)
		return 0
	} else {
		return lo.SumBy(seedHolderRecords, func(r *SeedHolderRecord) int { return len(r.Ids) })
	}
}

func (c *IndexerClient) GetSeasonNodeHolderData(endTimestamp int64) ([]*SeasonSBTRecord, error) {
	endpoint := fmt.Sprintf("%s/snapshot/%s/%s/%d", c.ApiBase, internal.SeasonSBTContractType, internal.SeasonSBTContractAddr, endTimestamp)
	log.Debug().Msgf("Try to get seed holder data, endpoint is %s", endpoint)

	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got error response from indexer endpoint: status code: %d, resp: %+v", resp.StatusCode, resp)
	}

	var respData []*SeasonSBTRecord
	err = json.NewDecoder(resp.Body).Decode(&respData)
	if err != nil {
		return nil, err
	}

	return respData, nil
}

func (c *IndexerClient) GetCurrentSeasonNodeCount(seasonNumberStr string) int {
	nodeSbtHolderRecords, err := c.GetSeasonNodeHolderData(time.Now().UTC().Unix())

	if err != nil {
		log.Error().Msgf("get node SBT holder info error: %+v", err)
		return 0
	} else {
		resultCount := 0
		for _, record := range nodeSbtHolderRecords {
			if lo.Contains(record.Ids, seasonNumberStr) {
				resultCount += 1
			}
		}
		return resultCount
	}
}
