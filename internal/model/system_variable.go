package model

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/allegro/bigcache/v3"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
)

type SystemVariable struct {
	Name     string `gorm:"uniqIndex"`
	NumValue int
	StrValue string
}

func getStrVal(db *gorm.DB, name string) (string, error) {
	log.Debug().Msgf("get string value request for name %s", name)
	cacheKey := fmt.Sprintf("sys_var_cache_%s", name)
	cachedValBytes, err := storage.GetCachedData(cacheKey)
	if err == nil {
		log.Debug().Msgf("get string value return %q", cachedValBytes)
		return string(cachedValBytes), nil
	}

	log.Debug().Msgf("load value from cache error: %+v, try to get from DB", err)
	var sysVar SystemVariable
	err = db.Model(&SystemVariable{}).Where("name = ?", name).First(&sysVar).Error
	if err != nil {
		log.Error().Msgf("get string value error from db error: %+v", err)
		return "", err
	}

	err = storage.StoreCachedData(cacheKey, []byte(sysVar.StrValue))
	if err != nil {
		log.Error().Msgf("store value to cache error: %+v", err)
	}

	return sysVar.StrValue, nil
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
		if err = tx.Model(&SystemVariable{}).Where("name='sip'").Limit(1).Pluck("num_value", &sipValue).Error; err != nil {
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

const MetaforoInfoVariableName = "metaforo_access_info"

// GetMetaforoData returns metaforo group name, group ID and admin token as map[string]string from DB
func GetMetaforoData(db *gorm.DB) (map[string]string, error) {
	log.Debug().Msgf("try to get metaforo access data")
	metaforoInfo := make(map[string]string)

	cachedVal, err := storage.GetCachedData(MetaforoInfoVariableName)

	if err != nil && !errors.Is(err, bigcache.ErrEntryNotFound) {
		log.Error().Msgf("get metaforo info returns error: %+v", err)
		return nil, err
	} else if err == nil {
		log.Debug().Msgf("get metaforo info return %q", cachedVal)
		err = json.Unmarshal(cachedVal, &metaforoInfo)
		if err != nil {
			log.Error().Msgf("parse metaforo cache value error: %+v, reload from DB", err)
		} else {
			log.Debug().Msgf("unmarshalled metaforo info: %+v", metaforoInfo)
			return metaforoInfo, nil
		}
	}

	// No cached value or parse cache value error, get from DB
	log.Debug().Msgf("load metaforo data from DB")
	var mfRecords []*SystemVariable
	err = db.Model(&SystemVariable{}).Where("name like ?", "metaforo_%").Find(&mfRecords).Error
	log.Error().Msgf("%+v", err)
	if err != nil {
		log.Error().Msgf("get metaforo access token from DB error: %+v", err)
		return nil, err
	} else if len(mfRecords) == 0 {
		err = errors.New("no metaforo info records found")
		log.Error().Msgf(err.Error())
		return nil, err
	} else {
		log.Debug().Msgf("get metaforo related records from DB: %+v", mfRecords)
		for _, mfRcd := range mfRecords {
			metaforoInfo[mfRcd.Name] = mfRcd.StrValue
		}
		log.Debug().Msgf("rebuilt metaforo info from DB: %+v", metaforoInfo)

		mfInfoBytes, err := json.Marshal(metaforoInfo)
		if err != nil {
			log.Warn().Msgf("marshal metaforo data error: %+v, no cache will be updaed", err)
			return metaforoInfo, nil
		}

		err = storage.StoreCachedData(MetaforoInfoVariableName, mfInfoBytes)
		if err != nil {
			log.Warn().Msgf("write metaforo info to cache error: %+v", err)
		}
		return metaforoInfo, nil
	}
}

func UpdateMetaforoAdminToken(db *gorm.DB, adminToken string) error {
	log.Debug().Msgf("update metaforo admin token to %s", adminToken)
	err := db.Model(&SystemVariable{}).Where("name = ?", internal.SysVarMfAdminToken).Update("str_value", adminToken).Error
	if err != nil {
		log.Error().Msgf("update metaforo admin token error: %+v", err)
		return err
	} else {
		log.Debug().Msgf("updated metaforo admin token to %s", adminToken)
	}

	// Invalidate cache
	err = storage.InvalidCache(MetaforoInfoVariableName)
	if err != nil {
		log.Error().Msgf("invalid metaforo info cache error: %+v", err)
		return err
	}
	err = storage.InvalidCache(internal.SysVarMfAdminToken)
	if err != nil {
		log.Error().Msgf("invalid metaforo info cache error: %+v", err)
		return err
	}
	return nil
}

func GetSeeAuthPk(db *gorm.DB) (string, error) {
	pkVal, err := getStrVal(db, internal.SysVarSeeAuthPk)
	if err != nil {
		log.Error().Msgf("get see auth pk error: %+v", err)
		return "", err
	} else {
		return pkVal, nil
	}
}
