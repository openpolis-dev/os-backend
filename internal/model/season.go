package model

import (
	"time"

	"github.com/theseed-labs/os-backend/internal"
	"gorm.io/gorm"
)

type Season struct {
	ID uint `gorm:"primaryKey"`

	Name string `gorm:"uniqueIndex:season_name size:32"`

	// Numeric index for the season, will be used to calculate latest credits in current season
	Idx uint `gorm:"uniqueIndex:season_idx size:32"`

	// Whether mint reward has been confirmed and the timestamp of confirmation
	MintRewardConfirmed   bool
	MintRewardConfirmedAt int64
	MintRewardAppBundleId uint // app bundle id saves application for this season's reward application

	// Whether seed data snapshot has been taken and the timestamp of snapshot
	SeedSnapshotSaved     bool
	SeedSnapshotAt        int64
	SeedSnapshotSubmitter string // submitter for the seed snapshot

	// StartAt and EndAt saves epoch second to avoid complex logic of timezone
	StartAt int64 `gorm:"index"`
	EndAt   int64 `gorm:"index"`
}

// GetCurrentSeason query season table to get current season record.
// The logic is finding out first record with start_at field earlier than current local time.
// This query does not create season record if not existing since the data should be created in InitDB function
func GetCurrentSeason(db *gorm.DB) (*Season, error) {
	now := time.Now().In(internal.ProjectTimezone).Unix()
	var currSeason Season
	err := db.Model(&Season{}).
		Where("start_at < ?", now).
		Order("start_at desc").
		First(&currSeason).Error

	if err != nil {
		return nil, err
	}
	return &currSeason, nil
}

func MustGetCurrentSeason(db *gorm.DB) *Season {
	season, err := GetCurrentSeason(db)
	if err != nil {
		panic(err)
	}

	return season
}

func GetSeasonsByName(db *gorm.DB, seasonNameList []string) ([]*Season, error) {
	var seasons []*Season
	err := db.Model(&Season{}).
		Where("name IN ?", seasonNameList).
		Find(&seasons).Error

	if err != nil {
		return nil, err
	}
	return seasons, nil
}
