package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/theseed-labs/os-backend/internal/config"
)

type (
	SubmitPaymentRequestReq struct {
		Rows               []*CreatePaymentRequestData `json:"rows"`
		CategoryId         int                         `json:"category_id"`
		CategoryName       string                      `json:"category_name"`
		CategoryProperties []*CategoryProperty         `json:"category_properties"`
	}
	CreatePaymentRequestData struct {
		Recipient               string `json:"recipient"`                 // 接收人
		Amount                  string `json:"amount"`                    // 资产数量
		Decimals                int    `json:"decimals"`                  // 资产 token 精度， 如 6
		CurrencyName            string `json:"currency_name"`             // 资产 token 名称，如 USDT
		CurrencyContractAddress string `json:"currency_contract_address"` // 资产 token 合约地址
	}
	CategoryProperty struct {
		Name   string `json:"name"`
		Type   string `json:"type"`
		Values string `json:"values"`
	}
)

// SubmitToQuickAccounting
//
// budgetSource 预算来源， 如 Xx项目
// session 季度名称，如 S5
// item 事项
// comment 备注
// applicant 申请人
// applyComment 申请说明
// reviewer 审核人
// reviewDate 审核时间
func SubmitToQuickAccounting(rows []*CreatePaymentRequestData, budgetSource, session, item, comment, applicant, applyComment, reviewer, reviewDate string, config *config.Config) error {
	categoryProperties := make([]*CategoryProperty, 8)
	categoryProperties[0] = &CategoryProperty{Name: "预算来源", Type: "Text", Values: budgetSource}
	categoryProperties[1] = &CategoryProperty{Name: "季度", Type: "Text", Values: session}
	categoryProperties[2] = &CategoryProperty{Name: "事项", Type: "Text", Values: item}
	categoryProperties[3] = &CategoryProperty{Name: "备注", Type: "Text", Values: comment}
	categoryProperties[4] = &CategoryProperty{Name: "申请人", Type: "Text", Values: applicant}
	categoryProperties[5] = &CategoryProperty{Name: "申请说明", Type: "Text", Values: applyComment}
	categoryProperties[6] = &CategoryProperty{Name: "审核人", Type: "Text", Values: reviewer}
	categoryProperties[7] = &CategoryProperty{Name: "审核时间", Type: "Text", Values: reviewDate}

	req := SubmitPaymentRequestReq{
		Rows:               rows,
		CategoryId:         config.QuickAccounting.CategoryId,
		CategoryName:       config.QuickAccounting.CategoryName,
		CategoryProperties: categoryProperties,
	}
	body, _ := json.Marshal(req)

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
