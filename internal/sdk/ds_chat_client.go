package sdk

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/common"
)

type DsChatResponse struct {
	ApiKey string `json:"apiKey"`
}

type DsChatClient struct {
	ApiBase string `json:"api_base"`
	AuthKey string `json:"auth_key"`
}

var dsChatClient *DsChatClient

func InitDsChatClient(apiBase string, authKey string) error {
	dsChatClient = &DsChatClient{
		ApiBase: apiBase,
		AuthKey: authKey,
	}
	return nil
}

func GetDsChatClient() *DsChatClient {
	return dsChatClient
}

func (c *DsChatClient) Auth(wallet string) (*DsChatResponse, error) {
	_wallet := common.FormatUserWallet(wallet)
	endpoint := fmt.Sprintf("%s/os/auth/%s", c.ApiBase, _wallet)
	log.Debug().Msgf("update spp profile, endpoint %s", endpoint)

	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.AuthKey))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	res := DsChatResponse{}
	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func (c *DsChatClient) Refersh(wallet string) (*DsChatResponse, error) {
	_wallet := common.FormatUserWallet(wallet)
	endpoint := fmt.Sprintf("%s/os/refresh/%s", c.ApiBase, _wallet)
	log.Debug().Msgf("update spp profile, endpoint %s", endpoint)

	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.AuthKey))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	res := DsChatResponse{}
	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}
