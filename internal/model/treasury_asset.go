package model

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
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

	AssetName    string          `json:"asset_name"`
	TotalAmount  decimal.Decimal `json:"total_amount" sql:"type:decimal(20,8);"`
	RemainAmount decimal.Decimal `json:"remain_amount" sql:"type:decimal(20,8);"`

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
	var creditTotal, creditRemain, tokenTotal, tokenRemain decimal.Decimal
	for _, detailedRcd := range r.DetailedRecords {
		if detailedRcd.BudgetType == BudgetTypeCredit {
			creditTotal = creditTotal.Add(detailedRcd.TotalAmount)
			creditRemain = creditRemain.Add(detailedRcd.RemainAmount)
		} else if detailedRcd.BudgetType == BudgetTypeToken {
			tokenTotal = tokenTotal.Add(detailedRcd.TotalAmount)
			tokenRemain = tokenRemain.Add(detailedRcd.RemainAmount)
		}
	}

	return &TreasuryAssetsResponse{
		ID:                 r.ID,
		QuarterNum:         r.QuarterNum,
		CreditTotalAmount:  creditTotal,
		CreditUsedAmount:   creditTotal.Sub(creditRemain),
		CreditRemainAmount: creditRemain,
		TokenTotalAmount:   tokenTotal,
		TokenUsedAmount:    tokenTotal.Sub(tokenRemain),
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

// GetOrCreateCQDetailedRecord creates detailed record for current quarter if not existing, then return the record to invoker
// The second return value indicates whether the record returned in first param is new created or existing data
func (*treasuryAssetHelper) GetOrCreateCQDetailedRecord(db *gorm.DB, treasuryRecordId uint, budgetType BudgetType, assetName string, totalAmount decimal.Decimal) (*TreasuryDetailedRecord, bool, error) {
	r := TreasuryDetailedRecord{}
	rslt := db.Where(TreasuryDetailedRecord{
		TreasuryAssetID: treasuryRecordId,
		BudgetType:      budgetType,
		AssetName:       assetName,
	}).Attrs(TreasuryDetailedRecord{
		TotalAmount:  totalAmount,
		RemainAmount: totalAmount,
	}).FirstOrInit(&r)

	if rslt.Error != nil {
		return nil, false, rslt.Error
	} else {
		if rslt.RowsAffected > 0 {
			return &r, false, nil
		} else {
			// RowsAffected == 0, record not existing so create it
			err := db.Save(&r).Error
			if err != nil {
				return nil, false, rslt.Error
			} else {
				return &r, true, nil
			}
		}
	}
}

// UpsertCQTreasuryDetailedRecord creates treasury detailed record and related create audit log
func (*treasuryAssetHelper) UpsertCQTreasuryDetailedRecord(db *gorm.DB, budgetType BudgetType, assetName string, totalAmount decimal.Decimal, userWallet string) error {
	cqRcd, err := TreasuryAssetHelper.GetOrCreateCurrQuarterRecord(db)
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		r, newRecord, err := TreasuryAssetHelper.GetOrCreateCQDetailedRecord(tx, cqRcd.ID, budgetType, assetName, totalAmount)
		if err != nil {
			return err
		}

		if !newRecord {
			// Record found, need to update the total amount and used amount
			usedAmount := r.TotalAmount.Sub(r.RemainAmount)
			r.TotalAmount = totalAmount
			r.RemainAmount = totalAmount.Sub(usedAmount)
			err = tx.Save(&r).Error
			if err != nil {
				return err
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
func (*treasuryAssetHelper) WithdrawTreasureAsset(db *gorm.DB, budgetType BudgetType, assetName string, deltaValue decimal.Decimal, userWallet string, auditMsg string) error {
	return TreasuryAssetHelper.ChangeCQTreasuryAssetValue(db, budgetType, assetName, deltaValue, userWallet, auditMsg)
}

// DepositTreasureAsset save asset back to treasury record
func (*treasuryAssetHelper) DepositTreasureAsset(db *gorm.DB, budgetType BudgetType, assetName string, deltaValue decimal.Decimal, userWallet string, auditMsg string) error {
	return TreasuryAssetHelper.ChangeCQTreasuryAssetValue(db, budgetType, assetName, deltaValue.Neg(), userWallet, auditMsg)
}

// ChangeCQTreasuryAssetValue update asset value for current quarter treasury record, the value passed in deltaValue allows both positive and negative value
// For positive value, the remain amount will be decreased while the negative means remain amount will be increased
func (*treasuryAssetHelper) ChangeCQTreasuryAssetValue(db *gorm.DB, budgetType BudgetType, assetName string, deltaValue decimal.Decimal, userWallet string, auditMsg string) error {
	cqRcd, err := TreasuryAssetHelper.GetOrCreateCurrQuarterRecord(db)
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// Search by treasury asset id and budget type, and init the record if not found
		r, _, err := TreasuryAssetHelper.GetOrCreateCQDetailedRecord(tx, cqRcd.ID, budgetType, assetName, decimal.Zero)
		if err != nil {
			return err
		}

		r.RemainAmount = r.RemainAmount.Sub(deltaValue)
		tx.Save(&r)

		return tx.Create(&TreasuryAuditLog{
			TreasuryDetailedRecordID: r.ID,
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
