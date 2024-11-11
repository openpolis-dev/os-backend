package data_srv

import "github.com/shopspring/decimal"

type MintResult struct {
	TotalSeasonCreditWithoutMint decimal.Decimal
	TotalMetaforoCredits         decimal.Decimal
	UserCredits                  map[string]UserCreditRecord
	ActivateWalletCount          int
	DetailRecords                []*CreditDetail
	MintRewardData               map[string]string
}
