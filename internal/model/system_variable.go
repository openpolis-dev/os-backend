package model

import (
	"errors"

	"github.com/allegro/bigcache/v3"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
)

type SystemVariable struct {
	Name     string
	NumValue int
	StrValue string
}

func GetNextSipValue(db *gorm.DB) (int, error) {
	log.Debug().Msgf("get next sip value request")
	var sipValue []int
	err := db.Transaction(func(tx *gorm.DB) error {
		err := tx.Model(&SystemVariable{}).Where("name='sip'").Update("num_value", gorm.Expr("num_value + ?", 1)).Error
		if err != nil {
			log.Error().Msgf("update sip value error: %+v", err)
			return err
		}
		if err = tx.Model(&SystemVariable{}).Limit(1).Pluck("num_value", &sipValue).Error; err != nil {
			log.Error().Msgf("update sip value error: %+v", err)
			return err
		}
		return nil
	})

	if err != nil {
		log.Error().Msgf("get sip value error: %+v", err)
		return 0, err
	} else if len(sipValue) < 1 {
		log.Error().Msgf("no sip value found")
		return 0, err
	}

	log.Debug().Msgf("get next sip value return %d", sipValue[0])
	return sipValue[0], nil
}
func RollbackSipValueByOne(db *gorm.DB) (int, error) {
	log.Debug().Msgf("rollback sip value by 1")
	var sipValue []int
	err := db.Transaction(func(tx *gorm.DB) error {
		err := tx.Model(&SystemVariable{}).Where("name='sip'").Update("num_value", gorm.Expr("num_value - ?", 1)).Error
		if err != nil {
			log.Error().Msgf("rollback sip value error: %+v", err)
			return err
		}
		if err = tx.Model(&SystemVariable{}).Limit(1).Pluck("num_value", &sipValue).Error; err != nil {
			log.Error().Msgf("rollback sip value error: %+v", err)
			return err
		}
		return nil
	})

	if err != nil {
		log.Error().Msgf("get sip value error: %+v", err)
		return 0, err
	} else if len(sipValue) < 1 {
		log.Error().Msgf("no sip value found")
		return 0, err
	}

	log.Debug().Msgf("rollbacked sip value return %d", sipValue[0])
	return sipValue[0], nil
}

const MetaforoAccessTokenVariableName = "metaforo_access_token"

// GetMetaforoAccessToken returns metaforo admin token saved in DB
func GetMetaforoAccessToken(db *gorm.DB) (string, error) {
	log.Debug().Msgf("get metaforo access token request")
	cachedVal, err := storage.GetCachedData(MetaforoAccessTokenVariableName)
	if err != nil && !errors.Is(err, bigcache.ErrEntryNotFound) {
		log.Error().Msgf("get metaforo access token error: %+v", err)
		return "", err
	} else if err == nil {
		log.Debug().Msgf("get metaforo access token return %s", cachedVal)
		return string(cachedVal), nil
	}

	// No cached value, get from DB
	log.Debug().Msgf("get metaforo access token from DB")
	var mfAccessTokenRecord SystemVariable
	err = db.Model(&SystemVariable{}).Where("name=?", MetaforoAccessTokenVariableName).First(&mfAccessTokenRecord).Error
	if err != nil {
		log.Error().Msgf("get metaforo access token error: %+v", err)
		return "", err
	} else {
		err = storage.StoreCachedData(MetaforoAccessTokenVariableName, []byte(mfAccessTokenRecord.StrValue))
		if err != nil {
			log.Warn().Msgf("get metaforo access token error: %+v", err)
		}
		return mfAccessTokenRecord.StrValue, nil
	}
}
