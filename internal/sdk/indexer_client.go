package sdk

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/common"
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

type Erc20SnapshotRecord struct {
	Wallet string          `json:"wallet"`
	Value  decimal.Decimal `json:"amount"`
}

type ComputeNodeSbt struct {
	Node int `json:"node"`
	Sbt  int `json:"sbt"`
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

func (c *IndexerClient) GetEnsoulSBTHolderInfo(endTimestamp int64) ([]*SeasonSBTRecord, error) {
	endpoint := fmt.Sprintf("%s/snapshot/%s/%s/%d", c.ApiBase, internal.EnsoulSbtContractType, internal.EnsoulSbtContractAddr, endTimestamp)
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
	nodeSbtHolderRecords, err := c.GetEnsoulSBTHolderInfo(time.Now().UTC().Unix())
	if err != nil {
		log.Error().Msgf("get node SBT holder info error: %+v", err)
		return []string{}, err
	} else {
		var csNodeWallet []string
		for _, record := range nodeSbtHolderRecords {
			if lo.Contains(record.Ids, seasonNumberStr) {
				csNodeWallet = append(csNodeWallet, record.Wallet)
			}
		}
		return csNodeWallet, nil
	}
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

	resp, err := http.Get(endpoint)
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

func (c *IndexerClient) GetUserCurrentScrAmount(userWallet string) (decimal.Decimal, error) {
	endpoint := fmt.Sprintf("%s/snapshot/%s/%s", c.ApiBase, internal.ScrContractType, internal.ScrContractAddr)
	log.Debug().Msgf("Try to get SCR amount, endpoint is %s", endpoint)

	resp, err := http.Get(endpoint)
	if err != nil {
		return decimal.Zero, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return decimal.Zero, fmt.Errorf("got error response from indexer endpoint: status code: %d, resp: %+v", resp.StatusCode, resp)
	}

	var respData []*Erc20SnapshotRecord
	err = json.NewDecoder(resp.Body).Decode(&respData)
	if err != nil {
		return decimal.Zero, err
	}

	formattedWallet := common.FormatUserWallet(userWallet)
	scrAmount := decimal.Zero
	for _, record := range respData {
		if record.Wallet == formattedWallet {
			scrAmount = record.Value
			break
		}
	}
	return scrAmount, nil
}
