package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const TableSnsChainRegistry = "sns_chain_registry"

// SnsChainRegistry stores on-chain SNS wallet ↔ name snapshots for offline export to seedao-api-server.
type SnsChainRegistry struct {
	ID        uint      `gorm:"primaryKey"`
	Wallet    string    `gorm:"type:varchar(42);uniqueIndex;not null"`
	SnsName   string    `gorm:"type:varchar(64);uniqueIndex;not null;column:sns_name"`
	Claimed   bool      `gorm:"not null;default:false"`
	SyncedAt  time.Time `gorm:"not null;column:synced_at"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (SnsChainRegistry) TableName() string {
	return TableSnsChainRegistry
}

type snsChainRegistryModel struct{}

var SnsChainRegistryModel snsChainRegistryModel

func (*snsChainRegistryModel) UpsertBatch(db *gorm.DB, rows []*SnsChainRegistry) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}

	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "wallet"}},
		DoUpdates: clause.AssignmentColumns([]string{"sns_name", "synced_at", "updated_at"}),
	}).CreateInBatches(rows, 200)
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}

func (*snsChainRegistryModel) FindAll(db *gorm.DB) ([]*SnsChainRegistry, error) {
	var rows []*SnsChainRegistry
	err := db.Table(TableSnsChainRegistry).Order("id asc").Find(&rows).Error
	return rows, err
}

func (*snsChainRegistryModel) FindByWallet(db *gorm.DB, wallet string) (*SnsChainRegistry, error) {
	var row SnsChainRegistry
	err := db.Table(TableSnsChainRegistry).Where("wallet = ?", wallet).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (*snsChainRegistryModel) Count(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Table(TableSnsChainRegistry).Count(&count).Error
	return count, err
}
