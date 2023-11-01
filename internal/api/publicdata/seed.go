package publicdata

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
)

type seed struct {
	TotalSupply string `json:"total_supply"`
}

var seedCache dataCache[http.Client, seed]

//var seedCache dataCache[ethclient.Client, seed]
//
//func SeedDataFromChain(ctx *gin.Context) {
//	_, cfg := api.ForContextDBAndConfig(ctx)
//
//	cacheLogic[ethclient.Client, seed](ctx, &seedCache, cfg.PublicData.CacheInSeconds, func() (*ethclient.Client, error) {
//		return ethclient.Dial(cfg.PublicData.MainnetRPC)
//	}, func() (*seed, error) {
//		totalSupply, err := contract.SeedTotalSupply(scrCache.client, cfg.PublicData.Contracts.Seed)
//		if err != nil {
//			return nil, err
//		}
//		return &seed{TotalSupply: totalSupply.Uint64()}, nil
//	})
//}

// SeedDataFromIndexer query SEED data from spp-indexer
// @Summary query SEED data from spp-indexer
// @Tags PublicData
// @Success 200 {object} scr
// @Router /public_data/contract/seed [get]
func SeedDataFromIndexer(ctx *gin.Context) {
	_, cfg := api.ForContextDBAndConfig(ctx)

	cacheLogic[http.Client, seed](ctx, &seedCache, cfg.PublicData.CacheInSeconds, func() (*http.Client, error) {
		return http.DefaultClient, nil
	}, func() (*seed, error) {
		resp, err := seedCache.client.Get(fmt.Sprintf("%s/insight/erc721/total_supply/%s", cfg.PublicData.SppIndexerHost, cfg.PublicData.Contracts.Seed))
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, errors.New("request SPP-Indexer failed")
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		_ = resp.Body.Close()

		var totalSupply InsightReply
		err = json.Unmarshal(body, &totalSupply)
		if err != nil {
			return nil, err
		}

		return &seed{TotalSupply: totalSupply.TotalSupply}, nil
	})
}
