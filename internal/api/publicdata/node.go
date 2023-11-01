package publicdata

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
)

type node struct {
	TotalSupply string `json:"total_supply"`
}

var nodeCache dataCache[http.Client, node]

// NodeDataFromIndexer query NODE data from spp-indexer
// @Summary query NODE data from spp-indexer
// @Tags PublicData
// @Success 200 {object} node
// @Router /public_data/contract/node [get]
func NodeDataFromIndexer(ctx *gin.Context) {
	_, cfg := api.ForContextDBAndConfig(ctx)

	cacheLogic[http.Client, node](ctx, &nodeCache, cfg.PublicData.CacheInSeconds, func() (*http.Client, error) {
		return http.DefaultClient, nil
	}, func() (*node, error) {
		resp, err := nodeCache.client.Get(fmt.Sprintf("%s/insight/erc1155/total_supply/%s", cfg.PublicData.SppIndexerHost, cfg.PublicData.Contracts.Node))
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

		return &node{TotalSupply: totalSupply.TotalSupply}, nil
	})
}
