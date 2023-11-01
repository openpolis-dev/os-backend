package publicdata

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
)

type scr struct {
	TotalSupply string `json:"total_supply"`
	//NodeTotalSupply *uint64 `json:"node,omitempty"`
}

var scrCache dataCache[http.Client, scr]

//var scrCache dataCache[ethclient.Client, scr]
//
//// SCRDataFromChain
//// `GET /public_data/contract/scr`
//func SCRDataFromChain(ctx *gin.Context) {
//	_, cfg := api.ForContextDBAndConfig(ctx)
//
//	cacheLogic[ethclient.Client, scr](ctx, &scrCache, cfg.PublicData.CacheInSeconds, func() (*ethclient.Client, error) {
//		return ethclient.Dial(cfg.PublicData.MainnetRPC)
//	}, func() (*scr, error) {
//		totalSupply, err := contract.SCRTotalSupply(scrCache.client, cfg.PublicData.Contracts.SCR)
//		if err != nil {
//			return nil, err
//		}
//		return &scr{TotalSupply: parseBigIntOnChainBalance(totalSupply, 18)}, nil
//	})
//}

// SCRDataFromIndexer query SCR data from spp-indexer
// @Summary query CR data from spp-indexer
// @Tags PublicData
// @Success 200 {object} scr
// @Router /public_data/contract/scr [get]
func SCRDataFromIndexer(ctx *gin.Context) {
	_, cfg := api.ForContextDBAndConfig(ctx)

	cacheLogic[http.Client, scr](ctx, &scrCache, cfg.PublicData.CacheInSeconds, func() (*http.Client, error) {
		return http.DefaultClient, nil
	}, func() (*scr, error) {
		resp, err := scrCache.client.Get(fmt.Sprintf("%s/insight/erc20/total_supply/%s", cfg.PublicData.SppIndexerHost, cfg.PublicData.Contracts.SCR))
		if err != nil {
			return nil, err
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

		return &scr{TotalSupply: totalSupply.TotalSupply}, nil
	})
}
