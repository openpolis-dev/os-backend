package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/theseed-labs/os-backend/internal/common"
)

// CacheStorage provides a general k-v cache with common interface
// Currently it only supports memory based cache for now, and can be extended to other backend with appropriated libs
type CacheStorage struct {
}

var cache *bigcache.BigCache

func InitCache() {
	cache, _ = bigcache.New(context.Background(), bigcache.DefaultConfig(10*time.Minute))
}

func GetCache() *bigcache.BigCache {
	if cache == nil {
		InitCache()
	}
	return cache
}

func GetCachedData(key string) ([]byte, error) {
	return GetCache().Get(key)
}

func StoreCachedData(key string, value []byte) error {
	return GetCache().Set(key, value)
}

func MetaforoRewardCacheKey(seasonIdx uint) string {
	return fmt.Sprintf("metaforo.reward.season.%d", seasonIdx)
}

func UserSeepassCacheKey(userWallet string) string {
	return fmt.Sprintf("seepass.user.%s", common.FormatUserWallet(userWallet))
}
