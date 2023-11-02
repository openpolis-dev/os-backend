package service

import (
	"time"

	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

// GetCurrentSeason query season table to get current season record.
// The logic is finding out first record with start_at field earlier than current local time.
// This query does not create season record if not existing since the data should be created in InitDB function
func GetCurrentSeason(db *gorm.DB) (*model.Season, error) {
	now := time.Now().In(internal.ProjectTimezone)
	var currSeason model.Season
	err := db.Model(&model.Season{}).
		Where("start_at < ?", now).
		Order("start_at desc").
		First(&currSeason).Error

	if err != nil {
		return nil, err
	}
	return &currSeason, nil
}

func GetSeasonByName(db *gorm.DB, seasonName string) (*model.Season, error) {
	var currSeason model.Season
	err := db.Model(&model.Season{}).
		Where("name = ", seasonName).
		First(&currSeason).Error

	if err != nil {
		return nil, err
	}
	return &currSeason, nil
}
