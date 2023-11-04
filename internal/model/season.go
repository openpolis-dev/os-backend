package model

import "time"

type Season struct {
	ID uint `gorm:"primaryKey"`

	Name string `gorm:"uniqueIndex size:32"`

	// Numeric index for the season, will be used to calculate latest credits in current season
	Idx uint `gorm:"uniqueIndex size:32"`

	// TODO: This field do not contains timezone info, need to review code about this
	StartAt time.Time `gorm:"index"`
	EndAt   time.Time
}
