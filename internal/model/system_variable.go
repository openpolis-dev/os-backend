package model

import (
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type SystemVariable struct {
	NextSip    int
	AdminToken string
}

func GetNextSipValue(db *gorm.DB) (int, error) {
	log.Debug().Msgf("get next sip value request")
	var sipValue []int
	err := db.Transaction(func(tx *gorm.DB) error {
		err := tx.Model(&SystemVariable{}).Where("1=1").Update("next_sip", gorm.Expr("next_sip + ?", 1)).Error
		if err != nil {
			log.Error().Msgf("update sip value error: %+v", err)
			return err
		}
		if err = tx.Model(&SystemVariable{}).Limit(1).Pluck("next_sip", &sipValue).Error; err != nil {
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

	return sipValue[0], nil
}
