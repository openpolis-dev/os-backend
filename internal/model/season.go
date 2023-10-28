package model

import "time"

type Season struct {
	ID uint `json:"id" gorm:"primaryKey"`

	Name string `json:"name" gorm:"uniq"`

	StartAt time.Time
	EndAt   time.Time
}
