package model

type Season struct {
	ID uint `gorm:"primaryKey"`

	Name string `gorm:"uniqueIndex size:32"`

	// Numeric index for the season, will be used to calculate latest credits in current season
	Idx uint `gorm:"uniqueIndex size:32"`

	// StartAt and EndAt saves epoch second to avoid complex logic of timezone
	StartAt int64 `gorm:"index"`
	EndAt   int64 `gorm:"index"`
}
