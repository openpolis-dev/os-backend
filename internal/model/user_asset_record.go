package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

// UserAssetRecord saves single asset balance of specified user.
// For each user and each asset, only one record is allowed in the database
// TODO: Add uniqueIndex to (UserWallet, AssetName). Currently some DB version do not support it.
type UserAssetRecord struct {
	ID               uint            `json:"id" gorm:"primaryKey"`
	UserWallet       string          `json:"user_wallet" gorm:"type:varchar(256)"`
	AssetName        string          `json:"asset_name" gorm:"type:varchar(64)"`          // asset name
	DealtAmount      decimal.Decimal `json:"dealt_amount" sql:"type:decimal(20,8);"`      // amount of asset that already dealt
	ProcessingAmount decimal.Decimal `json:"processing_amount" sql:"type:decimal(20,8);"` // amount of asset that still need confirmation
	CreatedAt        time.Time       `json:"-"`
	UpdatedAt        time.Time       `json:"-"`
	CreateTs         int64           `json:"create_ts" gorm:"index"`
	UpdateTs         int64           `json:"update_ts" gorm:"index"`
}

type userAssetRecordModel struct{}

var UserAssetRecordModel userAssetRecordModel

func (*userAssetRecordModel) FindWithUserWalletAndAssetProps(db *gorm.DB, userWallet string, assetName string) ([]*UserAssetRecord, error) {
	formattedUserWallet := common.FormatUserWallet(userWallet)

	// Create user record if not existing
	var r User
	userRslt := db.Where(User{Wallet: formattedUserWallet}).Attrs(User{
		CreatedAt: time.Now().In(internal.ProjectTimezone),
		UpdatedAt: time.Now().In(internal.ProjectTimezone),
		CreateTs:  GetCurrentUtcEpochSecond(),
		UpdateTs:  GetCurrentUtcEpochSecond(),
	}).FirstOrInit(&r)
	if userRslt.Error != nil {
		return nil, userRslt.Error
	} else if userRslt.RowsAffected == 0 {
		err := db.Save(&r).Error
		if err != nil {
			return nil, userRslt.Error
		}
	}

	querySeg := db.Where(&UserAssetRecord{UserWallet: formattedUserWallet, AssetName: assetName})
	return gormfind.Rows[UserAssetRecord](querySeg, nil)
}

func (*userAssetRecordModel) CreateOrUpdate(db *gorm.DB, userWallet string, assetName string, processingAmount, dealtAmount decimal.Decimal) error {
	assetRecords, err := UserAssetRecordModel.FindWithUserWalletAndAssetProps(db, userWallet, assetName)
	if err != nil {
		return err
	}

	if len(assetRecords) > 1 {
		return fmt.Errorf("user %s has more than one record for asset %s, please contract admin", userWallet, assetName)
	}

	if len(assetRecords) == 0 {
		return db.Save(&UserAssetRecord{
			UserWallet:       strings.TrimSpace(strings.ToLower(userWallet)),
			AssetName:        assetName,
			DealtAmount:      dealtAmount,
			ProcessingAmount: processingAmount,
		}).Error
	} else {
		assetRecords[0].DealtAmount = assetRecords[0].DealtAmount.Add(dealtAmount)
		assetRecords[0].ProcessingAmount = assetRecords[0].ProcessingAmount.Add(processingAmount)
		return db.Save(assetRecords).Error
	}
}

// Rollback extracts processing and dealt amount from records
func (*userAssetRecordModel) Rollback(db *gorm.DB, userWallet string, assetName string, processingAmount, dealtAmount decimal.Decimal) error {
	assetRecords, err := UserAssetRecordModel.FindWithUserWalletAndAssetProps(db, userWallet, assetName)
	if err != nil {
		return err
	}

	if (len(assetRecords) != 1) || (assetRecords[0].ProcessingAmount.Cmp(processingAmount) == 1) || (assetRecords[0].DealtAmount.Cmp(dealtAmount) == 1) {
		return fmt.Errorf("user %s has invalid record for asset %s, please contract admin", userWallet, assetName)
	}

	assetRecords[0].DealtAmount = assetRecords[0].DealtAmount.Sub(dealtAmount)
	assetRecords[0].ProcessingAmount = assetRecords[0].ProcessingAmount.Sub(processingAmount)

	return db.Save(assetRecords).Error
}

func (*userAssetRecordModel) CompleteAssetTransaction(db *gorm.DB, userWallet string, assetName string, amountToBeDealt decimal.Decimal) error {
	assetRecords, err := UserAssetRecordModel.FindWithUserWalletAndAssetProps(db, userWallet, assetName)
	if err != nil {
		return err
	}

	if (len(assetRecords) != 1) || (assetRecords[0].ProcessingAmount.Cmp(amountToBeDealt) == -1) {
		return fmt.Errorf("user %s has invalid record for asset %s, please contract admin", userWallet, assetName)
	}

	assetRecords[0].DealtAmount = assetRecords[0].DealtAmount.Add(amountToBeDealt)
	assetRecords[0].ProcessingAmount = assetRecords[0].ProcessingAmount.Sub(amountToBeDealt)

	return db.Save(assetRecords).Error
}
