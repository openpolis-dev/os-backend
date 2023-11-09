package model

type Season struct {
	ID uint `gorm:"primaryKey"`

	Name string `gorm:"uniqueIndex size:32"`

	// Numeric index for the season, will be used to calculate latest credits in current season
	Idx uint `gorm:"uniqueIndex size:32"`

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
