package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"
)

type Push struct {
	BaseURI string
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------
// ------ ------ ------ ------ ------ ------ ------ ------ ------

type PushToWalletsReq struct {
	Wallets []string    `json:"wallets"`
	Data    PushReqData `json:"data"`
}

type PushReqData struct {
	Title   map[string]string `json:"title"`
	Body    map[string]string `json:"body"`
	Payload map[string]string `json:"payload"`
}

func (p *Push) PushToWallets(ids []string, title map[string]string, body map[string]string, payload map[string]string) error {
	req := PushToWalletsReq{
		Wallets: ids,
		Data: PushReqData{
			Title:   title,
			Body:    body,
			Payload: payload,
		},
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	r, err := http.Post(fmt.Sprintf("%s/v1/push_to_wallets", p.BaseURI), "application/json", bytes.NewReader(data))
	if err != nil {
		log.Error().Msgf("Error when calling '/v1/push_to_wallets': %v", err)
		return err
	}

	if r.StatusCode != http.StatusOK {
		log.Error().Msgf("Error when calling '/v1/push_to_wallets', http code: %d", r.StatusCode)
		return fmt.Errorf("call push api failed with http code: %d", r.StatusCode)
	}
	_ = r.Body.Close()

	return nil
}

//func (n *Notification) PushGroup(groups []string, title string, body string, data map[string]any) error {
//	panic("implement me")
//}
//
//func (n *Notification) PushAll(title string, body string, data map[string]any) error {
//	panic("implement me")
//}
//
//func (n *Notification) SmsTo(phones []string, title string, body string) error {
//	panic("implement me")
//}
//
//func (n *Notification) EmailTo(emails []string, title string, body string) error {
//	panic("implement me")
//}
