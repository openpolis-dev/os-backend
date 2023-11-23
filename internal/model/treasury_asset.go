package model

import (
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"gorm.io/gorm"
)

type TreasuryAsset struct {
	ID uint `json:"id" gorm:"primaryKey"`

	DetailedRecords []TreasuryDetailedRecord `json:"detailed_records"`

	// Season information of treasury assets
	SeasonId uint    `json:"season_id"`
	Season   *Season `json:"season"`

	CreatedAt time.Time `json:"-" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"-" gorm:"autoUpdateTime"`
	CreateTs  int64     `json:"create_ts" gorm:"index"`
	UpdateTs  int64     `json:"update_ts" gorm:"index"`
}

type TreasuryDetailedRecord struct {
	ID              uint `json:"id" gorm:"primaryKey"`
	TreasuryAssetID uint `json:"treasury_asset_id"`

	AssetName    string          `json:"asset_name"`
	TotalAmount  decimal.Decimal `json:"total_amount" sql:"type:decimal(20,8);"`
	RemainAmount decimal.Decimal `json:"remain_amount" sql:"type:decimal(20,8);"`

	AuditLogs []TreasuryAuditLog `json:"audit_logs"`

	CreatedAt time.Time `json:"-" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"-" gorm:"autoUpdateTime"`
	CreateTs  int64     `json:"create_ts" gorm:"index"`
	UpdateTs  int64     `json:"update_ts" gorm:"index"`
}

type TreasuryAuditLog struct {
	ID uint `json:"id" gorm:"primaryKey"`

	TreasuryDetailedRecordID uint `json:"treasury_detailed_record_id"`

	AuditUserWallet string `json:"audit_user_wallet"`

	Action string `json:"action"`

	Message string `json:"message"`

	CreatedAt time.Time `json:"-" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"-" gorm:"autoUpdateTime"`
	CreateTs  int64     `json:"create_ts" gorm:"index"`
	UpdateTs  int64     `json:"update_ts" gorm:"index"`
}

func (r *TreasuryAsset) ToTreasuryAssetsResponse(db *gorm.DB) (*TreasuryAssetsResponse, error) {
	var creditTotal, creditUsed, tokenTotal, tokenUsed decimal.Decimal

	// Calculate total amount
	for _, detailedRcd := range r.DetailedRecords {
		if strings.HasPrefix(detailedRcd.AssetName, "USD") {
			tokenTotal = tokenTotal.Add(detailedRcd.TotalAmount)
		} else if strings.EqualFold(detailedRcd.AssetName, "SCR") {
			creditTotal = creditTotal.Add(detailedRcd.TotalAmount)
		} else {
			log.Warn().Msgf("non token or credit asset %s, ignore the treasury record: %+v", detailedRcd.AssetName, detailedRcd)
		}
	}

	appStates := []string{string(ApplicationStateOpen), ApplicationStateProcessing, ApplicationStateCompleted}
	appTypes := []string{string(ApplicationNewReward)}

	applications, err := GetCurrentSeasonApplications(db, appStates, appTypes)

	if err != nil {
		return nil, err
	}

	creditUsed = decimal.Zero
	tokenUsed = decimal.Zero
	for _, application := range applications {
		if strings.HasPrefix(application.AssetName, "USD") {
			// only calculate completed USD
			if application.State == ApplicationStateCompleted {
				tokenUsed = tokenUsed.Add(application.AssetAmount)
			}
		} else if strings.EqualFold(application.AssetName, "SCR") {
			// calculate processing and completed SCR
			creditUsed = creditUsed.Add(application.AssetAmount)
		} else {
			log.Warn().Msgf("non token or credit asset %s, ignore the application record: %+v", application.AssetName, application)
		}
	}

	return &TreasuryAssetsResponse{
		CreditTotalAmount: creditTotal,
		CreditUsedAmount:  creditUsed,
		TokenTotalAmount:  tokenTotal,
		TokenUsedAmount:   tokenUsed,
	}, nil
}

type treasuryAssetHelper struct{}

var TreasuryAssetHelper treasuryAssetHelper

// GetOrCreateCurrentSeasonRecord tries to get treasury record for current quarter, if not found a record with quarter num will be created and returned
func (*treasuryAssetHelper) GetOrCreateCurrentSeasonRecord(db *gorm.DB) (*TreasuryAsset, error) {
	currSeason, err := GetCurrentSeason(db)
	if err != nil {
		return nil, err
	}
	var r TreasuryAsset
	rslt := db.Preload("DetailedRecords").FirstOrInit(&r, TreasuryAsset{
		SeasonId:  currSeason.ID,
		CreateTs:  GetCurrentUtcEpochSecond(),
		CreatedAt: time.Now().In(internal.ProjectTimezone),
	})
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

// GetOrCreateCurrentSeasonDetailedRecord creates detailed record for current quarter if not existing, then return the record to invoker
// The second return value indicates whether the record returned in first param is new created or existing data
func (*treasuryAssetHelper) GetOrCreateCurrentSeasonDetailedRecord(db *gorm.DB, treasuryRecordId uint, assetName string, totalAmount decimal.Decimal) (*TreasuryDetailedRecord, bool, error) {
	r := TreasuryDetailedRecord{}
	rslt := db.Where(TreasuryDetailedRecord{
		TreasuryAssetID: treasuryRecordId,
		AssetName:       assetName,
	}).Attrs(TreasuryDetailedRecord{
		TotalAmount:  totalAmount,
		RemainAmount: totalAmount,
		CreatedAt:    time.Now().In(internal.ProjectTimezone),
		CreateTs:     GetCurrentUtcEpochSecond(),
		UpdatedAt:    time.Now().In(internal.ProjectTimezone),
		UpdateTs:     GetCurrentUtcEpochSecond(),
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

// UpsertCurrentSeasonTreasuryDetailedRecord creates treasury detailed record and related create audit log
func (*treasuryAssetHelper) UpsertCurrentSeasonTreasuryDetailedRecord(db *gorm.DB, assetName string, totalAmount decimal.Decimal, userWallet string) error {
	cqRcd, err := TreasuryAssetHelper.GetOrCreateCurrentSeasonRecord(db)
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		r, newRecord, err := TreasuryAssetHelper.GetOrCreateCurrentSeasonDetailedRecord(tx, cqRcd.ID, assetName, totalAmount)
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
func (*treasuryAssetHelper) WithdrawTreasureAsset(db *gorm.DB, assetName string, deltaValue decimal.Decimal, userWallet string, auditMsg string) error {
	return TreasuryAssetHelper.ChangeCQTreasuryAssetValue(db, assetName, deltaValue, userWallet, auditMsg)
}

// DepositTreasureAsset save asset back to treasury record
func (*treasuryAssetHelper) DepositTreasureAsset(db *gorm.DB, assetName string, deltaValue decimal.Decimal, userWallet string, auditMsg string) error {
	return TreasuryAssetHelper.ChangeCQTreasuryAssetValue(db, assetName, deltaValue.Neg(), userWallet, auditMsg)
}

// ChangeCQTreasuryAssetValue update asset value for current quarter treasury record, the value passed in deltaValue allows both positive and negative value
// For positive value, the remain amount will be decreased while the negative means remain amount will be increased
func (*treasuryAssetHelper) ChangeCQTreasuryAssetValue(db *gorm.DB, assetName string, deltaValue decimal.Decimal, userWallet string, auditMsg string) error {
	cqRcd, err := TreasuryAssetHelper.GetOrCreateCurrentSeasonRecord(db)
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// Search by treasury asset id and budget type, and init the record if not found
		r, _, err := TreasuryAssetHelper.GetOrCreateCurrentSeasonDetailedRecord(tx, cqRcd.ID, assetName, decimal.Zero)
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
