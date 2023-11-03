package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
)

type SeepassResponse struct {
	Sns      string      `json:"sns"`
	Wallet   string      `json:"wallet"`
	Avatar   interface{} `json:"avatar"`
	Email    interface{} `json:"email"`
	Nickname interface{} `json:"nickname"`
	Bio      interface{} `json:"bio"`
	Roles    []string    `json:"roles"`

	Scr struct {
		Amount       string `json:"amount"`
		ContractAddr string `json:"contract_addr"`
	} `json:"scr"`

	Level struct {
		CurrentLv      string `json:"current_lv"`
		NextLv         string `json:"next_lv"`
		ScrToNextLv    string `json:"scr_to_next_lv"`
		UpgradePercent string `json:"upgrade_percent"`
	} `json:"level"`

	Seed []struct {
		TokenId      string `json:"token_id"`
		ContractAddr string `json:"contract_addr"`
		ContractType string `json:"contract_type"`
		ImageUri     string `json:"image_uri"`
		TokenAmount  string `json:"token_amount"`
	} `json:"seed"`

	Sbt []struct {
		TokenId        string `json:"token_id"`
		ContractAddr   string `json:"contract_addr"`
		ContractType   string `json:"contract_type"`
		ImageUri       string `json:"image_uri"`
		TokenAmount    string `json:"token_amount"`
		CollectionName string `json:"collection_name"`
		Name           string `json:"name"`
		Symbol         string `json:"symbol"`
		Metadata       any    `json:"metadata"`
	} `json:"sbt"`

	SocialAccounts []interface{} `json:"social_accounts"`
}

type ProfileSocialAccount struct {
	Network  string `json:"network"`
	Identity string `json:"identity"`
	Verified bool   `json:"verified"`
}

type SppUpdateProfileRequest struct {
	Nickname string `json:"nickname"`
	Bio      string `json:"bio"`
	SnsName  string `json:"sns_name"`
	Avatar   string `json:"avatar"`
	Email    string `json:"email"`

	SocialAccounts []ProfileSocialAccount `json:"snRecords"`

	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type SppClient struct {
	ApiBase string `json:"api_base"`
}

var sppClient *SppClient

func InitSppClient(apiBase string) error {
	sppClient = &SppClient{
		ApiBase: apiBase,
	}
	return nil
}

func GetSppClient() *SppClient {
	return sppClient
}

func (c *SppClient) GetSeepassData(wallet string) (*SeepassResponse, error) {
	_wallet := strings.ToLower(wallet)
	endpoint := fmt.Sprintf("%s/seepass/%s", c.ApiBase, _wallet)
	log.Debug().Msgf("get SeePASS data, endpoint %s", endpoint)

	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got error response from seepass endpoint: status code: %d, resp: %+v", resp.StatusCode, resp)
	}

	seepassData := SeepassResponse{}
	err = json.NewDecoder(resp.Body).Decode(&seepassData)
	if err != nil {
		return nil, err
	}

	return &seepassData, nil
}

func (c *SppClient) UpdateProfile(wallet string, sppUpdateObject *SppUpdateProfileRequest) error {
	_wallet := strings.ToLower(wallet)
	endpoint := fmt.Sprintf("%s/profile/%s", c.ApiBase, _wallet)
	log.Debug().Msgf("update spp profile, endpoint %s, req data: %+v", endpoint, sppUpdateObject)
	payload, err := json.Marshal(sppUpdateObject)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	log.Debug().Msgf("profile data for user %s updated, post data: %+v, error: %+v", wallet, sppUpdateObject, resp)
	return nil
}
