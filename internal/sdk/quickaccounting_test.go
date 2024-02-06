package sdk

import (
	"testing"

	"github.com/theseed-labs/os-backend/internal/config"
)

func TestSubmitToQuickAccounting(t *testing.T) {
	c := config.Config{
		QuickAccounting: config.QuickAccounting{
			Url:          "https://qa-api.taoist.dev",
			WorkspaceId:  1,
			CategoryId:   1,
			CategoryName: "SeeDAO",
		},
	}

	rows := []*CreatePaymentRequestData{
		{
			Recipient:               "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
			Amount:                  "1000",
			Decimals:                6,
			CurrencyName:            "USDC",
			CurrencyContractAddress: "0xda9d4f9b69ac6C22e444eD9aF0CfC043b7a7f53f",
		},
		{
			Recipient:               "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
			Amount:                  "2000",
			Decimals:                6,
			CurrencyName:            "USDC",
			CurrencyContractAddress: "0xda9d4f9b69ac6C22e444eD9aF0CfC043b7a7f53f",
		},
		{
			Recipient:               "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
			Amount:                  "3000",
			Decimals:                6,
			CurrencyName:            "USDC",
			CurrencyContractAddress: "0xda9d4f9b69ac6C22e444eD9aF0CfC043b7a7f53f",
		},
	}
	err := SubmitToQuickAccounting(rows, "测试项目", "S5", "测试事项", "测试备注", "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266", "测试申请说明", "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266", "2024-02-05 22:22", &c)
	if err != nil {
		t.Fatal(err)
	}
}
