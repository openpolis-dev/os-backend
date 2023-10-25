package publicdata

import (
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/sdk/contract"
)

type seed struct {
	TotalSupply uint64 `json:"total_supply"`
}

var seedCache dataCache[ethclient.Client, seed]

// SeedData
// `GET /public_data/contract/seed`
func SeedData(ctx *gin.Context) {
	_, cfg := api.ForContextDBAndConfig(ctx)

	cacheLogic[ethclient.Client, seed](ctx, &seedCache, cfg.PublicData.CacheInSeconds, func() (*ethclient.Client, error) {
		return ethclient.Dial(cfg.PublicData.MainnetRPC)
	}, func() (*seed, error) {
		totalSupply, err := contract.SeedTotalSupply(scrCache.client, cfg.PublicData.Contracts.Seed)
		if err != nil {
			return nil, err
		}
		return &seed{TotalSupply: totalSupply.Uint64()}, nil
	})
}
