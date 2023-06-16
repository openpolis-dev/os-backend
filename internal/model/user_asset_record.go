package model

import (
	"fmt"
	"time"

	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

// UserAssetRecord saves single asset balance of specified user.
// For each user and each asset, only one record is allowed in the database
type UserAssetRecord struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	UserWallet       string    `json:"user_wallet"`
	AssetName        string    `json:"asset_name"`        // asset name
	DealtAmount      uint64    `json:"dealt_amount"`      // amount of asset that already dealt
	ProcessingAmount uint64    `json:"processing_amount"` // amount of asset that still need confirmation
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type userAssetRecordModel struct{}

var UserAssetRecordModel userAssetRecordModel

func (*userAssetRecordModel) FindWithUserWalletAndAssetName(db *gorm.DB, userWallet string, assetName string) ([]*UserAssetRecord, error) {
	querySeg := db.Model(&UserAssetRecord{UserWallet: userWallet, AssetName: assetName})
	return gormfind.Rows[UserAssetRecord](querySeg, nil)
}

func (*userAssetRecordModel) CreateOrUpdate(db *gorm.DB, userWallet string, assetName string, processingAmount, dealtAmount uint64) error {
	assetRecords, err := UserAssetRecordModel.FindWithUserWalletAndAssetName(db, userWallet, assetName)
	if err != nil {
		return err
	}

	if len(assetRecords) > 1 {
		return fmt.Errorf("user %s has more than one record for asset %s, please contract admin", userWallet, assetName)
	}

	if len(assetRecords) == 0 {
		return db.Save(&UserAssetRecord{
			UserWallet:       userWallet,
			AssetName:        assetName,
			DealtAmount:      dealtAmount,
			ProcessingAmount: processingAmount,
		}).Error
	} else {
		assetRecords[0].DealtAmount += dealtAmount
		assetRecords[0].ProcessingAmount += processingAmount
		return db.Save(assetRecords).Error
	}
}

// Rollback extracts processing and dealt amount from records
func (*userAssetRecordModel) Rollback(db *gorm.DB, userWallet string, assetName string, processingAmount, dealtAmount uint64) error {
	assetRecords, err := UserAssetRecordModel.FindWithUserWalletAndAssetName(db, userWallet, assetName)
	if err != nil {
		return err
	}

	if (len(assetRecords) != 1) || (assetRecords[0].ProcessingAmount > processingAmount) || (assetRecords[0].DealtAmount > dealtAmount) {
		return fmt.Errorf("user %s has invalid record for asset %s, please contract admin", userWallet, assetName)
	}

	assetRecords[0].DealtAmount -= dealtAmount
	assetRecords[0].ProcessingAmount -= processingAmount

	return db.Save(assetRecords).Error
}

func (*userAssetRecordModel) CompleteAssetTransaction(db *gorm.DB, userWallet string, assetName string, dealtAmount uint64) error {
	assetRecords, err := UserAssetRecordModel.FindWithUserWalletAndAssetName(db, userWallet, assetName)
	if err != nil {
		return err
	}

	if (len(assetRecords) != 1) || (assetRecords[0].ProcessingAmount > dealtAmount) {
		return fmt.Errorf("user %s has invalid record for asset %s, please contract admin", userWallet, assetName)
	}

	assetRecords[0].DealtAmount += dealtAmount
	assetRecords[0].ProcessingAmount -= dealtAmount

	return db.Save(assetRecords).Error
}
