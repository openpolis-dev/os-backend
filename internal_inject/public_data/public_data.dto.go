package publicdata_inject

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/sdk"
)

type dataCache[C any, D any] struct {
	client     *C
	data       *D
	updateTime int64 // unit is seconds
	lock       sync.Mutex
}

// read from remote data source or cache
func cacheLogic[C any, D any](ctx *gin.Context, cache *dataCache[C, D], cacheInSeconds int64, c func() (*C, error), d func() (*D, error)) {
	// get Mutex lock
	cache.lock.Lock()
	// unlock Mutex
	defer cache.lock.Unlock()

	var err error
	if cache.client == nil {
		log.Debug().Msgf("creating client...")

		cache.client, err = c()
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create client error detail"+err.Error())))
			return
		}
	}

	if time.Now().Unix()-cache.updateTime > cacheInSeconds {
		log.Debug().Msgf("querying data...")

		data, err := d()
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query data error")))
			return
		}

		cache.data = data
		cache.updateTime = time.Now().Unix()
	}

	ctx.JSON(http.StatusOK, api.Success(cache.data))
}

type discord struct {
	ID                       string `json:"id"`
	Name                     string `json:"name"`
	ApproximateMemberCount   int    `json:"approximate_member_count"`
	ApproximatePresenceCount int    `json:"approximate_presence_count"`
}

var discordCache dataCache[discordgo.Session, discord]

type (
	Vault struct {
		Wallets []*Wallet `json:"wallets"`
	}
	Wallet struct {
		ChainId   int    `json:"chainId"`
		Wallet    string `json:"wallet"`
		FiatTotal string `json:"fiatTotal"`
		Threshold int    `json:"threshold"`
		Owners    int    `json:"owners"`
	}
)

var vaultCache dataCache[http.Client, Vault]

type safeResponse struct {
	Threshold int   `json:"threshold"`
	Owners    []any `json:"owners"`
}

type safeBalanceResponse struct {
	FiatTotal string `json:"fiatTotal"`
}

type SeasonProposals struct {
	Link     string `json:"link"`
	Season   string `json:"season"`
	Category string `json:"category"`
	Tile     string `json:"title"`
	Create   int    `json:"create"`
}
