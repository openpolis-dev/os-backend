package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/theseed-labs/os-backend/internal/config"
)

type QAInput struct {
	Recipient               string
	Amount                  string
	Decimals                int
	CurrencyName            string
	CurrencyContractAddress string

	// budgetSource 预算来源， 如 Xx项目
	// session 季度名称，如 S5
	// item 事项
	// comment 备注
	// applicant 申请人
	// applyComment 申请说明
	// reviewer 审核人
	// reviewDate 审核时间
	BudgetSource string
	Session      string
	Item         string
	Comment      string
	Applicant    string
	ApplyComment string
	Reviewer     string
	ReviewDate   string
}

type (
	submitPaymentRequestReq struct {
		Rows []*createPaymentRequestData `json:"rows"`
	}
	createPaymentRequestData struct {
		Recipient               string              `json:"recipient"`
		Amount                  string              `json:"amount"`
		Decimals                int                 `json:"decimals"`
		CurrencyName            string              `json:"currency_name"`
		CurrencyContractAddress string              `json:"currency_contract_address"`
		CategoryId              int                 `json:"category_id"`
		CategoryName            string              `json:"category_name"`
		CategoryProperties      []*categoryProperty `json:"category_properties"`
	}
	categoryProperty struct {
		Name   string `json:"name"`
		Type   string `json:"type"`
		Values string `json:"values"`
	}
)

func SubmitToQuickAccounting(inputs []*QAInput, config *config.Config) error {
	if config.QuickAccounting.Url == "" {
		return nil // errors.New("'QuickAccounting.url' configuration is empty")
	}

	rows := make([]*createPaymentRequestData, len(inputs))
	for i, input := range inputs {
		categoryProperties := make([]*categoryProperty, 8)
		categoryProperties[0] = &categoryProperty{Name: "预算来源", Type: "Text", Values: input.BudgetSource}
		categoryProperties[1] = &categoryProperty{Name: "季度", Type: "Text", Values: input.Session}
		categoryProperties[2] = &categoryProperty{Name: "事项", Type: "Text", Values: input.Item}
		categoryProperties[3] = &categoryProperty{Name: "备注", Type: "Text", Values: input.Comment}
		categoryProperties[4] = &categoryProperty{Name: "申请人", Type: "Text", Values: input.Applicant}
		categoryProperties[5] = &categoryProperty{Name: "申请说明", Type: "Text", Values: input.ApplyComment}
		categoryProperties[6] = &categoryProperty{Name: "审核人", Type: "Text", Values: input.Reviewer}
		categoryProperties[7] = &categoryProperty{Name: "审核时间", Type: "Text", Values: input.ReviewDate}

		rows[i] = &createPaymentRequestData{
			Recipient:               input.Recipient,
			Amount:                  input.Amount,
			Decimals:                input.Decimals,
			CurrencyName:            input.CurrencyName,
			CurrencyContractAddress: input.CurrencyContractAddress,
			CategoryId:              config.QuickAccounting.CategoryId,
			CategoryName:            config.QuickAccounting.CategoryName,
			CategoryProperties:      categoryProperties,
		}
	}

	body, _ := json.Marshal(&submitPaymentRequestReq{rows})

	resp, err := http.Post(fmt.Sprintf("%s/payment_request_share/__direct__/direct_submit/%d", config.QuickAccounting.Url, config.QuickAccounting.WorkspaceId), "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed, status code: %d", resp.StatusCode)
	}

	return nil
}
