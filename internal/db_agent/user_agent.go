package db_agent

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/storage"
)

const userAvatarCacheKeyPrefix = "user_avatar_"

func GetUser(wallet string) (*model.User, error) {
	var err error
	agent := GetDbAgent()

	var user model.User
	if err = agent.db.Model(model.User{}).Where("wallet = ?", common.FormatUserWallet(wallet)).First(&user).Error; err != nil {
		log.Error().Msgf("Get user %s error: %+v", wallet, err)
		return nil, err
	}

	return &user, nil
}

func GetUserAvatar(wallet string) string {
	var err error
	userAvatarCacheKey := fmt.Sprintf("%s%s", userAvatarCacheKeyPrefix, common.FormatUserWallet(wallet))

	cachedAvatar, err := storage.GetCachedData(userAvatarCacheKey)
	if err == nil {
		return string(cachedAvatar)
	}

	log.Warn().Msgf("Get cached avatar for user %s error: %+v", wallet, err)

	userRcd, err := GetUser(wallet)

	if err != nil {
		log.Error().Msgf("Get user %s error: %+v", wallet, err)
		return ""
	}

	if err = storage.StoreCachedData(userAvatarCacheKey, []byte(userRcd.Avatar)); err != nil {
		log.Error().Msgf("Store cached avatar for user %s error: %+v", wallet, err)
		return ""
	}

	return userRcd.Avatar
}
