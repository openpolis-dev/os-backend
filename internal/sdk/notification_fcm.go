package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"
)

type FCM struct {
	BaseURI string
	Token   string
}

func NewFCM(baseURI string, token string) Pusher {
	return &FCM{baseURI, token}
}

// PushToWallets
// even `payload`'s type is `map[string]any`, but FCM only support `map[string]string`
func (f *FCM) PushToWallets(ids []string, title map[string]string, body map[string]string, payload map[string]any) error {
	req := &PushToWalletsReq{
		Wallets: ids,
		Data: PushReqData{
			Title:   title,
			Body:    body,
			Payload: payload,
		},
	}

	return f.doReq("/v1/push_to_wallets", req)
}

// PushAll
// even `payload`'s type is `map[string]any`, but FCM only support `map[string]string`
func (f *FCM) PushAll(title map[string]string, body map[string]string, payload map[string]any) error {
	req := &PushReqData{
		Title:   title,
		Body:    body,
		Payload: payload,
	}

	return f.doReq("/v1/push_to_all", req)
}

func (f *FCM) doReq(path string, reqParam any) error {
	data, err := json.Marshal(reqParam)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s%s", f.BaseURI, path), bytes.NewReader(data))
	if err != nil {
		log.Error().Msgf("Error when calling '%s', error: %v", path, err)
		return err
	}

	req.Header.Add("Token", f.Token)

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
