package model

type CommonBudgetSource struct {
	ID    uint   `json:"id" gorm:"primaryKey"`
	Name  string `json:"name"`
	State string `json:"state"`

	CreateTs int64 `json:"create_ts" gorm:"index"`
	UpdateTs int64 `json:"update_ts" gorm:"index"`
}
