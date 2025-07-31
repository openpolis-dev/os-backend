package asset_records_inject

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
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
func (s *AssetRecordsService) CreateTransfer(fromUser, toUser, assetName string, amount decimal.Decimal, comment string) (*model.UserAssetTransferLog, error) {
	upperAssetName := strings.ToUpper(assetName)
	log.Debug().Msgf("CreateTransfer: fromUser=%s, toUser=%s, assetName=%s, amount=%s, comment=%s", fromUser, toUser, upperAssetName, amount, comment)
	// Set default asset name to "see" if empty
	if upperAssetName == "" {
		upperAssetName = DefaultAssetName
	}
	// Validate that from and to users are different
	if fromUser == toUser {
		return nil, errors.New(ErrSameUserTransfer)
	}

	// Validate asset amount is positive
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, errors.New(ErrInvalidAmount)
	}

	// Check if from user has sufficient balance
	fromUserRecords, err := model.UserAssetRecordModel.FindWithUserWalletAndAssetProps(s.db, fromUser, upperAssetName)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrCheckingBalance, err)
	}

	if len(fromUserRecords) == 0 {
		return nil, errors.New(ErrNoAssetRecords)
	}

	availableBalance := fromUserRecords[0].DealtAmount
	if availableBalance.Cmp(amount) < 0 {
		return nil, errors.New(ErrInsufficientBalance)
	}

	var transferLog *model.UserAssetTransferLog

	// Execute all operations within a database transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Create transfer log
		var createErr error
		transferLog, createErr = model.UserAssetTransferLogModel.Create(tx, fromUser, toUser, upperAssetName, amount, comment)
		if createErr != nil {
			return fmt.Errorf("%s: %w", ErrCreatingTransfer, createErr)
		}

		// Deduct amount from from user's dealt amount
		if updateErr := model.UserAssetRecordModel.CreateOrUpdate(tx, fromUser, upperAssetName, decimal.Zero, amount.Neg()); updateErr != nil {
			model.UserAssetTransferLogModel.UpdateResult(tx, transferLog.ID, model.TransferResultFailed)
			return fmt.Errorf("%s: %w", ErrUpdatingFromUser, updateErr)
		}

		// Add amount to to user's dealt amount
		if updateErr := model.UserAssetRecordModel.CreateOrUpdate(tx, toUser, upperAssetName, decimal.Zero, amount); updateErr != nil {
			model.UserAssetTransferLogModel.UpdateResult(tx, transferLog.ID, model.TransferResultFailed)
			return fmt.Errorf("%s: %w", ErrUpdatingToUser, updateErr)
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
