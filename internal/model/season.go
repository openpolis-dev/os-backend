package model

import "time"

type Season struct {
	ID uint `gorm:"primaryKey"`

	Name string `gorm:"uniqueIndex"`

	// TODO: This field do not contains timezone info, need to review code about this
	StartAt time.Time `gorm:"index"`
	EndAt   time.Time
}
