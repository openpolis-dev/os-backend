package publicdata_inject

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/theseed-labs/os-backend/internal/config"
	"gorm.io/gorm"
)

type PublicDataService struct {
	// inject

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`
}

// https://safe-client.safe.global/v1/chains/1/safes/0x7FdA3253c94F09fE6950710E5273165283f8b283
func (s *PublicDataService) Safe(chainId int, wallet string, client *http.Client) (int, int, error) {
	resp, err := client.Get(fmt.Sprintf("https://safe-client.safe.global/v1/chains/%d/safes/%s", chainId, wallet))
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, errors.New("request Safe api failed")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, err
	}
	var safeResp safeResponse
	err = json.Unmarshal(body, &safeResp)
	if err != nil {
		return 0, 0, err
	}

	return safeResp.Threshold, len(safeResp.Owners), nil
}

// https://safe-client.safe.global/v1/chains/1/safes/0x7FdA3253c94F09fE6950710E5273165283f8b283/balances/usd?trusted=true
func (s *PublicDataService) SafeBalance(chainId int, wallet string, client *http.Client) (string, error) {
	resp, err := client.Get(fmt.Sprintf("https://safe-client.safe.global/v1/chains/%d/safes/%s/balances/usd?trusted=true", chainId, wallet))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("request Safe api failed")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var safeResp safeBalanceResponse
	err = json.Unmarshal(body, &safeResp)
	if err != nil {
		return "", err
	}

	return safeResp.FiatTotal, nil
}
