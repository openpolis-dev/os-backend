package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/rs/zerolog/log"
)

type sendScrRequest struct {
	Applicant string   `json:"applicant"`
	Accounts  []string `json:"accounts"`
	Amounts   []string `json:"amounts"`
}

// Copy from task_manager/application_task.go to avoid import cycle
type autoTransferScrParam struct {
	Applicant string                 `json:"applicant"`
	Items     []*autoTransferScrItem `json:"items"`
}

type autoTransferScrItem struct {
	ApplicationId uint   `json:"application_id"`
	TargetWallet  string `json:"target_wallet"`
	ScrAmount     string `json:"scr_amount"`
}

func SendScr(apiEndpoint, apiKey, apiSecret, taskParamStr, applicant string) ([]byte, error) {
	reqData := sendScrRequest{
		Applicant: applicant,
		Accounts:  []string{},
		Amounts:   []string{},
	}

	var taskParam autoTransferScrParam
	err := json.Unmarshal([]byte(taskParamStr), &taskParam)
	if err != nil {
		log.Error().Msgf("parse auto transfer SCR params error: %+v", err)
		return nil, err
	}

	for _, item := range taskParam.Items {
		reqData.Accounts = append(reqData.Accounts, item.TargetWallet)
		reqData.Amounts = append(reqData.Amounts, item.ScrAmount)
	}

	reqBytes, err := json.Marshal(reqData)
	if err != nil {
		log.Error().Msgf("marshal send SCR request error: %+v", err)
		return nil, err
	}

	// http.Post with headers
	req, err := http.NewRequest("POST", apiEndpoint, bytes.NewBuffer(reqBytes))
	if err != nil {
		log.Error().Msgf("create send SCR request error: %+v", err)
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", apiKey)
	req.Header.Set("Api-Secret", apiSecret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error().Msgf("send SCR request error: %+v", err)
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got error response from SCR service: status code: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
