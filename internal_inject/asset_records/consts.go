package asset_records_inject

// Error messages
const (
	ErrSameUserTransfer      = "from_user and to_user cannot be the same"
	ErrInvalidAmount         = "amount must be greater than zero"
	ErrNoAssetRecords        = "from_user has no asset records for this asset"
	ErrInsufficientBalance   = "insufficient balance"
	ErrTransactionNotFound   = "transaction record not found"
	ErrCheckingBalance       = "error checking from user balance"
	ErrCreatingTransfer      = "error creating transfer log"
	ErrUpdatingFromUser    = "error updating from user processing amount"
	ErrUpdatingToUser      = "error updating to user processing amount"
	ErrUpdatingResult      = "error updating transfer result"
	ErrCompletingFromUser  = "error completing from user transaction"
	ErrCompletingToUser    = "error completing to user transaction"
)

// Status values
const (
	StatusSuccess = "success"
)

// Route constants
const (
	BaseRoutePath = "/asset_trade"
)

// Pagination defaults
const (
	DefaultPageSize    = 20
	MaxPageSize        = 100
	DefaultPageNumber  = 1
)

// Field names
const (
	FieldTransferID = "transfer_id"
	FieldStatus     = "status"
	FieldID         = "id"
	FieldFromUser   = "from_user"
	FieldToUser     = "to_user"
	FieldAssetName  = "asset_name"
	FieldAmount     = "amount"
	FieldTimestamp  = "transaction_ts"
	FieldResult     = "result"
	FieldComment    = "comment"
)

// Default values
const (
	DefaultTransferAssetName = "SEE"
)