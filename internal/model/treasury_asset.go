package model

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

type TreasuryAsset struct {
	ID uint `json:"id" gorm:"primaryKey"`

	QuarterNum string `json:"quarter_num"` // Quarter num, the format is yyyy0[1234]

	DetailedRecords []TreasuryDetailedRecord `json:"detailed_records"`

	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

type TreasuryDetailedRecord struct {
	ID              uint `json:"id" gorm:"primaryKey"`
	TreasuryAssetID uint `json:"treasury_asset_id"`

	BudgetType BudgetType `json:"budget_type"`

	AssetName    string `json:"asset_name"`
	TotalAmount  uint64 `json:"total_amount"`
	RemainAmount int64  `json:"remain_amount"`

	AuditLogs []TreasuryAuditLog `json:"audit_logs"`

	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

type TreasuryAuditLog struct {
	ID uint `json:"id" gorm:"primaryKey"`

	TreasuryDetailedRecordID uint `json:"treasury_detailed_record_id"`

	AuditUserWallet string `json:"audit_user_wallet"`

	Action string `json:"action"`

	Message string `json:"message"`

	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (r *TreasuryAsset) ToTreasuryAssetsResponse() *TreasuryAssetsResponse {
	var creditTotal, tokenTotal uint64
	var creditRemain, tokenRemain int64
	for _, detailedRcd := range r.DetailedRecords {
		if detailedRcd.BudgetType == BudgetTypeCredit {
			creditTotal += detailedRcd.TotalAmount
			creditRemain += detailedRcd.RemainAmount
		} else if detailedRcd.BudgetType == BudgetTypeToken {
			tokenTotal += detailedRcd.TotalAmount
			tokenRemain += detailedRcd.RemainAmount
		}
	}

	return &TreasuryAssetsResponse{
		ID:                 r.ID,
		QuarterNum:         r.QuarterNum,
		CreditTotalAmount:  creditTotal,
		CreditRemainAmount: creditRemain,
		TokenTotalAmount:   tokenTotal,
		TokenRemainAmount:  tokenRemain,
	}
}

type treasuryAssetHelper struct{}

var TreasuryAssetHelper treasuryAssetHelper

// GetOrCreateCurrQuarterRecord tries to get treasury record for current quarter, if not found a record with quarter num will be created and returned
func (*treasuryAssetHelper) GetOrCreateCurrQuarterRecord(db *gorm.DB) (*TreasuryAsset, error) {
	var r TreasuryAsset
	rslt := db.Preload("DetailedRecords").FirstOrInit(&r, TreasuryAsset{QuarterNum: getCurrentQuarterNum()})
	if rslt.Error != nil {
		return nil, rslt.Error
	} else if rslt.RowsAffected == 0 {
		err := db.Save(&r).Error
		if err != nil {
			return nil, rslt.Error
		}
	}
	return &r, nil
}

// GetCurrQuarterRecord gets the treasury record of current quarter and return not found error if no record found
func (*treasuryAssetHelper) GetCurrQuarterRecord(db *gorm.DB) (*TreasuryAsset, error) {
	var r TreasuryAsset
	err := db.Where(&r, TreasuryAsset{QuarterNum: getCurrentQuarterNum()}).First(&r).Error
	return &r, err
}

// UpsertCQTreasuryDetailedRecord creates treasury detailed record and related create audit log
func (*treasuryAssetHelper) UpsertCQTreasuryDetailedRecord(db *gorm.DB, budgetType BudgetType, assetName string, totalAmount uint64, userWallet string) error {
	cqRcd, err := TreasuryAssetHelper.GetCurrQuarterRecord(db)
	if err != nil {
		return err
	}

	r := TreasuryDetailedRecord{}
	return db.Transaction(func(tx *gorm.DB) error {
		// Search by treasury asset id and budget type, and init the record if not found
		rslt := tx.Where(TreasuryDetailedRecord{
			TreasuryAssetID: cqRcd.ID,
			BudgetType:      budgetType,
			AssetName:       assetName,
		}).Attrs(TreasuryDetailedRecord{
			TotalAmount:  totalAmount,
			RemainAmount: int64(totalAmount),
		}).FirstOrInit(&r)

		if rslt.Error != nil {
			return rslt.Error
		} else if rslt.RowsAffected > 0 {
			// Record found, need to update the total amount
			r.TotalAmount = totalAmount
			err = tx.Save(&r).Error
			if err != nil {
				return err
			}
		} else if rslt.RowsAffected == 0 {
			err := db.Save(&r).Error
			if err != nil {
				return rslt.Error
			}
		}

		return tx.Create(&TreasuryAuditLog{
			TreasuryDetailedRecordID: r.ID,
			AuditUserWallet:          userWallet,
			Action:                   "create",
		}).Error
	})
}

// WithdrawTreasureAsset get asset from treasury record
func (*treasuryAssetHelper) WithdrawTreasureAsset(db *gorm.DB, budgetType BudgetType, assetName string, deltaValue uint64, userWallet string, auditMsg string) error {
	return TreasuryAssetHelper.ChangeCQTreasuryAssetValue(db, budgetType, assetName, int64(deltaValue), userWallet, auditMsg)
}

// DepositTreasureAsset save asset back to treasury record
func (*treasuryAssetHelper) DepositTreasureAsset(db *gorm.DB, budgetType BudgetType, assetName string, deltaValue uint64, userWallet string, auditMsg string) error {
	return TreasuryAssetHelper.ChangeCQTreasuryAssetValue(db, budgetType, assetName, int64(-deltaValue), userWallet, auditMsg)
}

// ChangeCQTreasuryAssetValue update asset value for current quarter treasury record, the value passed in deltaValue allows both positive and negative value
// For positive value, the remain amount will be decreased while the negative means remain amount will be increased
func (*treasuryAssetHelper) ChangeCQTreasuryAssetValue(db *gorm.DB, budgetType BudgetType, assetName string, deltaValue int64, userWallet string, auditMsg string) error {
	cqRcd, err := TreasuryAssetHelper.GetCurrQuarterRecord(db)
	if err != nil {
		return err
	}

	r := TreasuryDetailedRecord{}
	return db.Transaction(func(tx *gorm.DB) error {
		// Search by treasury asset id and budget type, and init the record if not found
		detailedRcd := TreasuryDetailedRecord{
			TreasuryAssetID: cqRcd.ID,
			BudgetType:      budgetType,
			AssetName:       assetName,
		}

		err = tx.Where(&detailedRcd).First(&r).Error
		if err != nil {
			return err
		}

		r.RemainAmount -= deltaValue
		tx.Save(&r)

		return tx.Create(&TreasuryAuditLog{
			TreasuryDetailedRecordID: detailedRcd.ID,
			AuditUserWallet:          userWallet,
			Action:                   "update",
			Message:                  auditMsg,
		}).Error
	})
}

func monthToSeasonIndex(m time.Month) int {
	switch m {
	case time.January, time.February, time.March:
		return 1
	case time.April, time.May, time.June:
		return 2
	case time.July, time.August, time.September:
		return 3
	case time.October, time.November, time.December:
		return 4
	}
	return 0
}

func getCurrentQuarterNum() string {
	year, month, _ := time.Now().Date()
	quarterIdx := monthToSeasonIndex(month)

	return fmt.Sprintf("%d%02d", year, quarterIdx)
}
