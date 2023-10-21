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
	Token   string
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
	req := &PushToWalletsReq{
		Wallets: ids,
		Data: PushReqData{
			Title:   title,
			Body:    body,
			Payload: payload,
		},
	}

	return p.doReq("/v1/push_to_wallets", req)
}

func (p *Push) PushAll(title map[string]string, body map[string]string, payload map[string]string) error {
	req := &PushReqData{
		Title:   title,
		Body:    body,
		Payload: payload,
	}

	return p.doReq("/v1/push_to_all", req)
}

func (p *Push) doReq(path string, reqParam any) error {
	data, err := json.Marshal(reqParam)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s%s", p.BaseURI, path), bytes.NewReader(data))
	if err != nil {
		log.Error().Msgf("Error when calling '%s', error: %v", path, err)
		return err
	}

	req.Header.Add("Token", p.Token)

	r, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error().Msgf("Error when calling '%s', error: %v", path, err)
		return err
	}

	if r.StatusCode != http.StatusOK {
		log.Error().Msgf("Error when calling '%s', http code: %d", path, r.StatusCode)
		return fmt.Errorf("call push api failed with http code: %d", r.StatusCode)
	}
	_ = r.Body.Close()

	return nil
}
