package publicdata

import (
	"net/http"
	"sync"
	"time"

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
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
	}

	if time.Now().Unix()-cache.updateTime > cacheInSeconds {
		log.Debug().Msgf("querying data...")

		data, err := d()
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}

		cache.data = data
		cache.updateTime = time.Now().Unix()
	}

	ctx.JSON(http.StatusOK, api.Success(cache.data))
}
