package publicdata

import (
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/api"
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
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
	}

	if time.Now().Unix()-discordCache.updateTime > cacheInSeconds {
		log.Debug().Msgf("querying data...")

		data, err := d()
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
		//log.Debug().Msgf("%+v", d)

		cache.data = data
		cache.updateTime = time.Now().Unix()
	}

	ctx.JSON(http.StatusOK, api.Success(cache.data))
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------

// convert big.Int to float64
func parseBigIntOnChainBalance(balance *big.Int, decimal int64) float64 {
	deci := new(big.Int).Exp(big.NewInt(10), big.NewInt(decimal), nil) // 10^n
	y := new(big.Float).SetInt(deci)

	z := new(big.Float)
	z.SetInt(balance)
	_ = z.Quo(z, y) // `Quo` sets z to the rounded quotient x/y and returns z

	// big.Float convert to float64
	r, _ := z.Float64()
	return r
}
