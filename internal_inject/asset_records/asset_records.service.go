package asset_records_inject

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AssetRecordsService handles business logic for asset transfers
type AssetRecordsService struct {
	db *gorm.DB
}

// NewAssetRecordsService creates a new instance of AssetRecordsService
func NewAssetRecordsService(db *gorm.DB) *AssetRecordsService {
	return &AssetRecordsService{db: db}
}

// CreateTransfer creates a new asset transfer with all necessary validations
// This implementation uses pessimistic locking within transaction to prevent race conditions
func (s *AssetRecordsService) CreateTransfer(fromUser, toUser, assetName string, amount decimal.Decimal, comment string) (*model.UserAssetTransferLog, error) {
	// Set default asset name to "see" if empty
	if assetName == "" {
		assetName = DefaultTransferAssetName
	}
	upperAssetName := strings.ToUpper(assetName)

	log.Debug().Msgf("CreateTransfer: fromUser=%s, toUser=%s, assetName=%s, amount=%s, comment=%s", fromUser, toUser, upperAssetName, amount, comment)

	// Validate that from and to users are different
	if fromUser == toUser {
		return nil, errors.New(ErrSameUserTransfer)
	}

	// Validate asset amount is positive
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, errors.New(ErrInvalidAmount)
	}

	var transferLog *model.UserAssetTransferLog

	// Execute all operations within a database transaction
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Create transfer log first
		var createErr error
		transferLog, createErr = model.UserAssetTransferLogModel.Create(tx, fromUser, toUser, upperAssetName, amount, comment)
		if createErr != nil {
			return fmt.Errorf("%s: %w", ErrCreatingTransfer, createErr)
		}

		// Lock and check from user's balance within transaction using SELECT FOR UPDATE
		var fromUserRecord model.UserAssetRecord
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "NOWAIT"}).
			Where("user_wallet = ? AND asset_name = ?", fromUser, upperAssetName).
			First(&fromUserRecord).Error

		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				model.UserAssetTransferLogModel.UpdateResult(tx, transferLog.ID, model.TransferResultFailed)
				return errors.New(ErrNoAssetRecords)
			}
			model.UserAssetTransferLogModel.UpdateResult(tx, transferLog.ID, model.TransferResultFailed)
			return fmt.Errorf("%s: %w", ErrCheckingBalance, err)
		}

		// Check if balance is sufficient
		if fromUserRecord.DealtAmount.Cmp(amount) < 0 {
			model.UserAssetTransferLogModel.UpdateResult(tx, transferLog.ID, model.TransferResultFailed)
			return errors.New(ErrInsufficientBalance)
		}

		// Atomically update balances using direct SQL
		// Deduct amount from from user
		result := tx.Model(&model.UserAssetRecord{}).
			Where("user_wallet = ? AND asset_name = ?", fromUser, upperAssetName).
			Update("dealt_amount", gorm.Expr("dealt_amount::decimal - ?", amount.String()))
		if result.Error != nil {
			model.UserAssetTransferLogModel.UpdateResult(tx, transferLog.ID, model.TransferResultFailed)
			return fmt.Errorf("%s: %w", ErrUpdatingFromUser, result.Error)
		}

		// Add amount to to user (create record if not exists)
		var toUserRecord model.UserAssetRecord
		err = tx.Where("user_wallet = ? AND asset_name = ?", toUser, upperAssetName).First(&toUserRecord).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create new record for to user
			if createErr := model.UserAssetRecordModel.CreateOrUpdate(tx, toUser, upperAssetName, amount, decimal.Zero); createErr != nil {
				model.UserAssetTransferLogModel.UpdateResult(tx, transferLog.ID, model.TransferResultFailed)
				return fmt.Errorf("%s: %w", ErrUpdatingToUser, createErr)
			}
		} else if err != nil {
			model.UserAssetTransferLogModel.UpdateResult(tx, transferLog.ID, model.TransferResultFailed)
			return fmt.Errorf("%s: %w", ErrUpdatingToUser, err)
		} else {
			// Update existing record
			result := tx.Model(&model.UserAssetRecord{}).
				Where("user_wallet = ? AND asset_name = ?", toUser, upperAssetName).
				Update("dealt_amount", gorm.Expr("dealt_amount::decimal + ?", amount.String()))
			if result.Error != nil {
				model.UserAssetTransferLogModel.UpdateResult(tx, transferLog.ID, model.TransferResultFailed)
				return fmt.Errorf("%s: %w", ErrUpdatingToUser, result.Error)
			}
		}

		// Update transfer result to success
		if updateErr := model.UserAssetTransferLogModel.UpdateResult(tx, transferLog.ID, model.TransferResultSuccess); updateErr != nil {
			return fmt.Errorf("%s: %w", ErrUpdatingResult, updateErr)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return transferLog, nil
}

// ListTransfers returns paginated asset transfer records with optional filters
func (s *AssetRecordsService) ListTransfers(page, size int, fromUser, toUser string) ([]*model.UserAssetTransferLog, int64, error) {
	return model.UserAssetTransferLogModel.ListPaginated(s.db, page, size, fromUser, toUser)
}

// GetTransferByID returns a single transfer record by ID
func (s *AssetRecordsService) GetTransferByID(id uint) (*model.UserAssetTransferLog, error) {
	return model.UserAssetTransferLogModel.GetByID(s.db, id)
}
