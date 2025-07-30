package asset_records_inject

import (
	"github.com/shopspring/decimal"
)

// CreateTransferRequest represents the request payload for creating a new asset transfer
type CreateTransferRequest struct {
	FromUser  string          `json:"from_user" binding:"required"`
	ToUser    string          `json:"to_user" binding:"required"`
	AssetName string          `json:"asset_name"`
	Amount    decimal.Decimal `json:"amount" binding:"required,gt=0"`
	Comment   string          `json:"comment"`
}

// TransferListQueryParams represents query parameters for listing transfers
type TransferListQueryParams struct {
	FromUser string `form:"from_user"`
	ToUser   string `form:"to_user"`
	Page     int    `form:"page,default=1"`
	Size     int    `form:"size,default=20"`
}

// TransferResponse represents the response format for asset transfer records
type TransferResponse struct {
	ID            uint            `json:"id"`
	FromUser      string          `json:"from_user"`
	ToUser        string          `json:"to_user"`
	AssetName     string          `json:"asset_name"`
	Amount        decimal.Decimal `json:"amount"`
	TransactionTs int64           `json:"transaction_ts"`
	Result        string          `json:"result"`
	Comment       string          `json:"comment"`
}
