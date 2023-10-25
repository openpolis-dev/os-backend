package publicdata

import (
	"github.com/theseed-labs/os-backend/internal/sdk/contract"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
)

type scr struct {
	TotalSupply float64 `json:"total_supply"`
	//NodeTotalSupply *uint64 `json:"node,omitempty"`
}

var scrCache dataCache[ethclient.Client, scr]

// SCRData
// `GET /public_data/contract/scr`
func SCRData(ctx *gin.Context) {
	_, cfg := api.ForContextDBAndConfig(ctx)

	cacheLogic[ethclient.Client, scr](ctx, &scrCache, cfg.PublicData.CacheInSeconds, func() (*ethclient.Client, error) {
		return ethclient.Dial(cfg.PublicData.MainnetRPC)
	}, func() (*scr, error) {
		totalSupply, err := contract.SCRTotalSupply(scrCache.client, cfg.PublicData.Contracts.SCR)
		if err != nil {
			return nil, err
		}
		return &scr{TotalSupply: parseBigIntOnChainBalance(totalSupply, 18)}, nil
	})
}
