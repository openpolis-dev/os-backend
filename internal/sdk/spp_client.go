package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
)

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
