package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

// UserAssetRecord saves single asset balance of specified user.
// For each user and each asset, only one record is allowed in the database
type UserAssetRecord struct {
	ID               uint            `json:"id" gorm:"primaryKey"`
	UserWallet       string          `json:"user_wallet" gorm:"type:varchar(256)"`
	AssetType        BudgetType      `json:"asset_type"`                                  // type of the asset, credit or token
	AssetName        string          `json:"asset_name"`                                  // asset name
	DealtAmount      decimal.Decimal `json:"dealt_amount" sql:"type:decimal(20,8);"`      // amount of asset that already dealt
	ProcessingAmount decimal.Decimal `json:"processing_amount" sql:"type:decimal(20,8);"` // amount of asset that still need confirmation
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type userAssetRecordModel struct{}

var UserAssetRecordModel userAssetRecordModel

func (*userAssetRecordModel) FindWithUserWalletAndAssetProps(db *gorm.DB, userWallet string, assetType BudgetType, assetName string) ([]*UserAssetRecord, error) {
	formattedUserWallet := strings.TrimSpace(strings.ToLower(userWallet))
	querySeg := db.Where(&UserAssetRecord{UserWallet: formattedUserWallet, AssetType: assetType, AssetName: assetName})
	return gormfind.Rows[UserAssetRecord](querySeg, nil)
}

func (*userAssetRecordModel) CreateOrUpdate(db *gorm.DB, userWallet string, assetType BudgetType, assetName string, processingAmount, dealtAmount decimal.Decimal) error {
	assetRecords, err := UserAssetRecordModel.FindWithUserWalletAndAssetProps(db, userWallet, assetType, assetName)
	if err != nil {
		return err
	}

	if len(assetRecords) > 1 {
		return fmt.Errorf("user %s has more than one record for asset type %s.%s, please contract admin", userWallet, assetName, assetType)
	}

	if len(assetRecords) == 0 {
		return db.Save(&UserAssetRecord{
			UserWallet:       strings.TrimSpace(strings.ToLower(userWallet)),
			AssetType:        assetType,
			AssetName:        assetName,
			DealtAmount:      dealtAmount,
			ProcessingAmount: processingAmount,
		}).Error
	} else {
		assetRecords[0].DealtAmount.Add(dealtAmount)
		assetRecords[0].ProcessingAmount.Add(processingAmount)
		return db.Save(assetRecords).Error
	}
}

// Rollback extracts processing and dealt amount from records
func (*userAssetRecordModel) Rollback(db *gorm.DB, userWallet string, assetType BudgetType, assetName string, processingAmount, dealtAmount decimal.Decimal) error {
	assetRecords, err := UserAssetRecordModel.FindWithUserWalletAndAssetProps(db, userWallet, assetType, assetName)
	if err != nil {
		return err
	}

	if (len(assetRecords) != 1) || (assetRecords[0].ProcessingAmount.Cmp(processingAmount) == 1) || (assetRecords[0].DealtAmount.Cmp(dealtAmount) == 1) {
		return fmt.Errorf("user %s has invalid record for asset type %s, please contract admin", userWallet, assetType)
	}

	assetRecords[0].DealtAmount = assetRecords[0].DealtAmount.Sub(dealtAmount)
	assetRecords[0].ProcessingAmount = assetRecords[0].ProcessingAmount.Sub(processingAmount)

	return db.Save(assetRecords).Error
}

func (*userAssetRecordModel) CompleteAssetTransaction(db *gorm.DB, userWallet string, assetType BudgetType, assetName string, amountToBeDealt decimal.Decimal) error {
	assetRecords, err := UserAssetRecordModel.FindWithUserWalletAndAssetProps(db, userWallet, assetType, assetName)
	if err != nil {
		return err
	}

	if (len(assetRecords) != 1) || (assetRecords[0].ProcessingAmount.Cmp(amountToBeDealt) == -1) {
		return fmt.Errorf("user %s has invalid record for asset type %s, please contract admin", userWallet, assetType)
	}

	assetRecords[0].DealtAmount = assetRecords[0].DealtAmount.Add(amountToBeDealt)
	assetRecords[0].ProcessingAmount = assetRecords[0].ProcessingAmount.Sub(amountToBeDealt)

	return db.Save(assetRecords).Error
}
