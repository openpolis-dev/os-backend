package model

type DataSource struct {
	ID       uint  `gorm:"primaryKey"`
	CreateTs int64 `gorm:"index"`
	UpdateTs int64 `gorm:"index"`

	Name        string
	Description string
	GqlCommand  string
}
