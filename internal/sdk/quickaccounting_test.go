package sdk

import (
	"testing"

	"github.com/theseed-labs/os-backend/internal/config"
)

func TestSubmitToQuickAccounting(t *testing.T) {
	c := config.Config{
		QuickAccounting: config.QuickAccounting{
			Url:          "https://dev-qa-api.taoist.dev",
			WorkspaceId:  1,
			CategoryId:   1,
			CategoryName: "SeeDAO",
		},
	}

	rows := []*QAInput{
		{
			Recipient:               "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
			Amount:                  "1000",
			Decimals:                6,
			CurrencyName:            "USDC",
			CurrencyContractAddress: "0xda9d4f9b69ac6C22e444eD9aF0CfC043b7a7f53f",
			BudgetSource:            "测试项目",
			Session:                 "S5",
			Item:                    "测试事项",
			Comment:                 "测试备注",
			Applicant:               "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
			ApplyComment:            "测试申请说明",
			Reviewer:                "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
			ReviewDate:              "2024-02-05 22:22:22",
		},
		{
			Recipient:               "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
			Amount:                  "2000",
			Decimals:                6,
			CurrencyName:            "USDC",
			CurrencyContractAddress: "0xda9d4f9b69ac6C22e444eD9aF0CfC043b7a7f53f",
			BudgetSource:            "测试项目",
			Session:                 "S5",
			Item:                    "测试事项",
			Comment:                 "测试备注",
			Applicant:               "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
			ApplyComment:            "测试申请说明",
			Reviewer:                "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
			ReviewDate:              "2024-02-05 22:22:22",
		},
		{
			Recipient:               "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
			Amount:                  "3000",
			Decimals:                6,
			CurrencyName:            "USDC",
			CurrencyContractAddress: "0xda9d4f9b69ac6C22e444eD9aF0CfC043b7a7f53f",
			BudgetSource:            "测试项目",
			Session:                 "S5",
			Item:                    "测试事项",
			Comment:                 "测试备注",
			Applicant:               "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
			ApplyComment:            "测试申请说明",
			Reviewer:                "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
			ReviewDate:              "2024-02-05 22:22:22",
		},
	}
	err := SubmitToQuickAccounting(rows, &c)
	if err != nil {
		t.Fatal(err)
	}
}
