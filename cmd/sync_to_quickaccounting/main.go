package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	osDSN := "host=localhost user=postgres password=3.1415work dbname=os-backend-prod port=5432"
	//qaDSN := "host=localhost user=postgres password=3.1415work dbname=quickaccounting port=5432"
	qaDSN := "host=43.133.7.129 user=postgres password=#CQe9La(@rS!3[ dbname=dashboard_test port=9527"

	osConn, err := gorm.Open(postgres.Open(osDSN))
	if err != nil {
		panic(err)
	}
	qaConn, err := gorm.Open(postgres.Open(qaDSN))
	if err != nil {
		panic(err)
	}

	err = qaConn.AutoMigrate(&PaymentRequest{}, &PaymentRequestItem{})
	if err != nil {
		return
	}

	// ----- ----- ----- ----- ----- ----- ----- -----
	// ----- ----- ----- ----- ----- ----- ----- -----
	sessionMap := make(map[uint]*model.Season)
	projMap := make(map[uint]*model.Project)
	guildMap := make(map[uint]*model.Guild)

	var sessions []*model.Season
	if err = osConn.Model(&model.Season{}).Find(&sessions).Error; err != nil {
		panic(err)
	}
	for _, session := range sessions {
		sessionMap[session.ID] = session
	}

	var projects []*model.Project
	if err = osConn.Model(&model.Project{}).Find(&projects).Error; err != nil {
		panic(err)
	}
	for _, proj := range projects {
		projMap[proj.ID] = proj
	}

	var guilds []*model.Guild
	if err = osConn.Model(&model.Guild{}).Find(&guilds).Error; err != nil {
		panic(err)
	}
	for _, guild := range guilds {
		guildMap[guild.ID] = guild
	}
	// ----- ----- ----- ----- ----- ----- ----- -----
	// ----- ----- ----- ----- ----- ----- ----- -----

	var applications []*model.Application
	if err = osConn.Model(&model.Application{}).Find(&applications).Error; err != nil { // .Limit(100)
		panic(err)
	}

	//
	workspaceId := uint(1)
	categoryId := uint(1)
	categoryName := "SeeDAO"
	now := time.Now().Format(time.DateTime)

	// create payment request
	request := PaymentRequest{
		WorkspaceId: workspaceId,
		Name:        "imported from os-backend",
		Status:      5,
	}
	if err = qaConn.Model(&PaymentRequest{}).Create(&request).Error; err != nil {
		panic(err)
	}

	// create payment request items
	appCount := len(applications)
	items := make([]*PaymentRequestItem, appCount)
	for i, app := range applications {
		season, ok := sessionMap[app.SeasonId]
		if !ok {

		}
		dc, exist := internal.AssertDecimalsAndContractAddr[app.AssetName]
		if !exist {

		}
		budgetSource := "unknown budget source"
		if app.EntityType == "guild" {
			budgetSource = guildMap[app.EntityId].Name
		} else if app.EntityType == "project" {
			budgetSource = projMap[app.EntityId].Name
		}

		categoryProperties := make([]*CategoryProperty, 8)
		categoryProperties[0] = &CategoryProperty{Name: "预算来源", Type: "Text", Values: budgetSource}
		categoryProperties[1] = &CategoryProperty{Name: "季度", Type: "Text", Values: season.Name}
		categoryProperties[2] = &CategoryProperty{Name: "事项", Type: "Text", Values: app.DetailedType}
		categoryProperties[3] = &CategoryProperty{Name: "备注", Type: "Text", Values: app.Comment}
		categoryProperties[4] = &CategoryProperty{Name: "申请人", Type: "Text", Values: app.Applicant}
		categoryProperties[5] = &CategoryProperty{Name: "申请说明", Type: "Text", Values: ""}
		categoryProperties[6] = &CategoryProperty{Name: "审核人", Type: "Text", Values: app.Applicant}
		categoryProperties[7] = &CategoryProperty{Name: "审核时间", Type: "Text", Values: now}
		properties, _ := json.Marshal(categoryProperties)

		items[i] = &PaymentRequestItem{
			WorkspaceId:             workspaceId,
			PaymentRequestId:        request.ID,
			Recipient:               app.TargetUserWallet,
			Amount:                  app.AssetAmount,
			Decimals:                uint(dc.Decimals),
			CurrencyName:            app.AssetName,
			CurrencyContractAddress: dc.Addr,
			CategoryId:              categoryId,
			CategoryName:            categoryName,
			CategoryProperties:      string(properties),
			SafeTxHash:              "",
			TxHash:                  "",
			TxTimestamp:             0,
			Status:                  5,
			Hide:                    false,
		}
	}

	batchSize := 10
	for i := 0; i < appCount/batchSize+1; i++ {
		start := i * batchSize
		end := (i + 1) * batchSize
		if end > appCount {
			end = appCount
		}
		fmt.Println(start, end)
		itemsPart := items[start:end]
		if err = qaConn.Model(&PaymentRequestItem{}).Create(&itemsPart).Error; err != nil {
			panic(err)
		}
	}
}

type (
	PaymentRequest struct {
		gorm.Model
		WorkspaceId uint `json:"workspace_id"`

		Name string `json:"name"`

		Status int `json:"status"`
	}
	PaymentRequestItem struct {
		gorm.Model
		WorkspaceId      uint `json:"workspace_id"`
		PaymentRequestId uint `json:"payment_request_id"`

		Recipient               string          `json:"recipient"`
		Amount                  decimal.Decimal `json:"amount"`
		Decimals                uint            `json:"decimals"`
		CurrencyName            string          `json:"currency_name"`
		CurrencyContractAddress string          `json:"currency_contract_address"`

		CategoryId         uint   `json:"category_id"`
		CategoryName       string `json:"category_name"`
		CategoryProperties string `json:"category_properties"`

		SafeTxHash string `json:"safe_tx_hash"` // identifier of safe.global

		TxHash      string `json:"tx_hash"` // transaction hash
		TxTimestamp int64  `json:"tx_timestamp"`

		Status int `json:"status"` // same as PaymentRequest.Status

		Hide bool `json:"hide"`
	}
)

type CategoryProperty struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Values string `json:"values"`
}
