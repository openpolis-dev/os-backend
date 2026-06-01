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

type ComputeNodeSbt struct {
	Node int `json:"node"`
	Sbt  int `json:"sbt"`
}

type IndexerClient struct {
	ApiBase    string       `json:"api_base"`
	httpClient *http.Client `json:"-"`
}

var indexerClient *IndexerClient

// InitIndexerClient configures the global indexer client. httpTimeout is applied to every outbound HTTP request; use 0 for default (55s).
func InitIndexerClient(apiBase string, httpTimeout time.Duration) error {
	if httpTimeout <= 0 {
		httpTimeout = 55 * time.Second
	}
	indexerClient = &IndexerClient{
		ApiBase: apiBase,
		httpClient: &http.Client{
			Timeout: httpTimeout,
		},
	}
	return nil
}

func GetIndexerClient() *IndexerClient {
	return indexerClient
}

func (c *IndexerClient) GetSeedHolderInfo(endTimestamp int64) ([]*SeedHolderRecord, error) {
	endpoint := fmt.Sprintf("%s/snapshot/%s/%s/%d", c.ApiBase, internal.SeedContractType, internal.SeedContractAddr, endTimestamp)
	log.Debug().Msgf("Try to get seed holder data, endpoint is %s", endpoint)

	resp, err := c.httpClient.Get(endpoint)
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

func (c *IndexerClient) GetEnsoulSBTHolderInfo(endTimestamp int64) ([]*SeasonSBTRecord, error) {
	return c.getEnsoulSBTHolderInfo(endTimestamp, c.httpClient)
}

func (c *IndexerClient) getEnsoulSBTHolderInfo(endTimestamp int64, client *http.Client) ([]*SeasonSBTRecord, error) {
	if client == nil {
		client = c.httpClient
	}
	endpoint := fmt.Sprintf("%s/snapshot/%s/%s/%d", c.ApiBase, internal.EnsoulSbtContractType, internal.EnsoulSbtContractAddr, endTimestamp)
	log.Debug().Msgf("Try to get ensoul SBT holder data, endpoint is %s", endpoint)

	resp, err := client.Get(endpoint)
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
	nodeSbtHolderRecords, err := c.GetEnsoulSBTHolderInfo(time.Now().UTC().Unix())

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

func (c *IndexerClient) GetCurrentSeasonNodeList(seasonNumberStr string) ([]string, error) {
	return c.GetCurrentSeasonNodeListWithTimeout(seasonNumberStr, 0)
}

// GetCurrentSeasonNodeListWithTimeout filters ensoul SBT holders by season token id.
// timeout 0 uses the client timeout from InitIndexerClient.
func (c *IndexerClient) GetCurrentSeasonNodeListWithTimeout(seasonNumberStr string, timeout time.Duration) ([]string, error) {
	client := c.httpClient
	if timeout > 0 {
		client = &http.Client{Timeout: timeout}
	}
	nodeSbtHolderRecords, err := c.getEnsoulSBTHolderInfo(time.Now().UTC().Unix(), client)
	if err != nil {
		return nil, err
	}
	var csNodeWallet []string
	for _, record := range nodeSbtHolderRecords {
		if lo.Contains(record.Ids, seasonNumberStr) {
			csNodeWallet = append(csNodeWallet, record.Wallet)
		}
	}
	return csNodeWallet, nil
}

func (c *IndexerClient) GetCurrentCityHallCount() int {
	nodeSbtHolderRecords, err := c.GetEnsoulSBTHolderInfo(time.Now().UTC().Unix())

	if err != nil {
		log.Error().Msgf("get cityhall holder info error: %+v", err)
		return 0
	} else {
		resultCount := 0
		for _, record := range nodeSbtHolderRecords {
			if lo.Contains(record.Ids, internal.CityHallTokenId) {
				resultCount += 1
			}
		}
		return resultCount
	}
}

func (c *IndexerClient) GetComputeNodeSbt() (*ComputeNodeSbt, error) {
	endpoint := fmt.Sprintf("%s/computenodesbt", c.ApiBase)
	log.Debug().Msgf("Try to get compute node SBT, endpoint is %s", endpoint)

	resp, err := c.httpClient.Get(endpoint)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got error response from indexer endpoint: status code: %d, resp: %+v", resp.StatusCode, resp)
	}

	var respData *ComputeNodeSbt
	err = json.NewDecoder(resp.Body).Decode(&respData)
	if err != nil {
		return nil, err
	}

	return respData, nil
}
