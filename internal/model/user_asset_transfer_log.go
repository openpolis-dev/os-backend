package model

import (
	"time"

	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal/common"
	"gorm.io/gorm"
)

// UserAssetTransferLog records asset transfers between users
// This table logs all asset transfer transactions
// from_user and to_user should point to different records in user table
// asset_name: the name of the asset being transferred
// amount: the quantity of the asset being transferred
// transaction_ts: timestamp when the transaction was initiated
// result: the final status of the transaction (success, failed, pending)
type UserAssetTransferLog struct {
	ID            uint            `json:"id" gorm:"primaryKey"`
	FromUser      string          `json:"from_user" gorm:"type:varchar(256);index"` // user wallet address of sender
	ToUser        string          `json:"to_user" gorm:"type:varchar(256);index"`   // user wallet address of receiver
	AssetName     string          `json:"asset_name" gorm:"type:varchar(64);index"` // asset name
	Amount        decimal.Decimal `json:"amount" sql:"type:decimal(20,8);"`         // amount of asset transferred
	TransactionTs int64           `json:"transaction_ts" gorm:"index"`              // timestamp when transaction was initiated
	Result        string          `json:"result" gorm:"type:varchar(20);index"`     // success, failed, pending
	Comment       string          `json:"comment" gorm:"type:varchar(500)"`         // optional comment for the transfer
	CreatedAt     time.Time       `json:"-"`
	UpdatedAt     time.Time       `json:"-"`
	CreateTs      int64           `json:"create_ts" gorm:"index"`
	UpdateTs      int64           `json:"update_ts" gorm:"index"`
}

// TransferResult constants
const (
	TransferResultSuccess = "success"
	TransferResultFailed  = "failed"
	TransferResultPending = "pending"
)

type userAssetTransferLogModel struct{}

var UserAssetTransferLogModel userAssetTransferLogModel

// Create creates a new asset transfer log record
func (*userAssetTransferLogModel) Create(db *gorm.DB, fromUser string, toUser string, assetName string, amount decimal.Decimal, comment string) (*UserAssetTransferLog, error) {
	fromUser = common.FormatUserWallet(fromUser)
	toUser = common.FormatUserWallet(toUser)

	// Ensure both users exist in the database
	var fromUserRecord, toUserRecord User

	// get from user and to user
	if err := db.Where(User{Wallet: fromUser}).First(&fromUserRecord).Error; err != nil {
		return nil, err
	}

	if err := db.Where(User{Wallet: toUser}).FirstOrCreate(&toUserRecord).Error; err != nil {
		return nil, err
	}

	transferLog := &UserAssetTransferLog{
		FromUser:      fromUser,
		ToUser:        toUser,
		AssetName:     assetName,
		Amount:        amount,
		TransactionTs: GetCurrentUtcEpochSecond(),
		Result:        TransferResultPending,
		Comment:       comment,
	}

	if err := db.Create(transferLog).Error; err != nil {
		return nil, err
	}

	return transferLog, nil
}

// UpdateResult updates the result of a transfer log
func (*userAssetTransferLogModel) UpdateResult(db *gorm.DB, id uint, result string) error {
	return db.Model(&UserAssetTransferLog{}).Where("id = ?", id).Updates(map[string]interface{}{
		"result": result,
	}).Error
}

// ListPaginated returns paginated transfer records with optional filters
func (*userAssetTransferLogModel) ListPaginated(db *gorm.DB, page, size int, fromUser, toUser string) ([]*UserAssetTransferLog, int64, error) {
	var logs []*UserAssetTransferLog
	var total int64

	query := db.Model(&UserAssetTransferLog{})

	if fromUser != "" {
		query = query.Where("from_user = ?", common.FormatUserWallet(fromUser))
	}

	if toUser != "" {
		query = query.Where("to_user = ?", common.FormatUserWallet(toUser))
	}

	// Count total records
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	if page > 0 && size > 0 {
		offset := (page - 1) * size
		query = query.Offset(offset).Limit(size)
	}

	// Order by transaction timestamp desc
	query = query.Order("transaction_ts DESC")

	if err := query.Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// GetByID returns a single transfer log by ID
func (*userAssetTransferLogModel) GetByID(db *gorm.DB, id uint) (*UserAssetTransferLog, error) {
	var log UserAssetTransferLog
	if err := db.Where("id = ?", id).First(&log).Error; err != nil {
		return nil, err
	}
	return &log, nil
}
