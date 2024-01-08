package publicdata

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
)

// https://safe-client.safe.global/v1/chains/1/safes/0x444C1Cf57b65C011abA9BaBEd05C6b13C11b03b5/balances/usd?trusted=true
// https://safe-client.safe.global/v1/chains/137/safes/0x4876eaD85CE358133fb80276EB3631D192196e24/balances/usd?trusted=true
//
// https://safe-client.safe.global/v1/chains/1/safes/0x7FdA3253c94F09fE6950710E5273165283f8b283

type (
	Vault struct {
		Wallets []*Wallet `json:"wallets"`
	}
	Wallet struct {
		ChainId   int    `json:"chainId"`
		Wallet    string `json:"wallet"`
		FiatTotal string `json:"fiatTotal"`
		Threshold int    `json:"threshold"`
		Owners    int    `json:"owners"`
	}
)

var vaultCache dataCache[http.Client, Vault]

// SafeVault returns the data of the safe vault
//
//	@summary	SafeVault returns the data of the safe vault
//	@tags		PublicData
//	@accept		json
//	@produce	json
//	@success	200	{object}	api.Reply{data=Vault}
//	@router		/public_data/safe_vault [get]
func SafeVault(ctx *gin.Context) {
	_, cfg := api.ForContextDBAndConfig(ctx)

	cacheLogic[http.Client, Vault](ctx, &vaultCache, cfg.PublicData.CacheInSeconds, func() (*http.Client, error) {
		return http.DefaultClient, nil
	}, func() (*Vault, error) {
		vault := &Vault{}

		wg := sync.WaitGroup{}
		wg.Add(len(cfg.PublicData.SafeVaults))
		for _, vv := range cfg.PublicData.SafeVaults {
			v := vv
			go func() {
				threshold, owners, err := safe(v.ChainId, v.Wallet, vaultCache.client)
				if err != nil {
					log.Error().Msgf("safe api failed: %s", err.Error())
				}

				fiatTotal, err := safeBalance(v.ChainId, v.Wallet, vaultCache.client)
				if err != nil {
					log.Error().Msgf("safe balance api failed: %s", err.Error())
				}

				vault.Wallets = append(vault.Wallets, &Wallet{
					ChainId:   v.ChainId,
					Wallet:    v.Wallet,
					FiatTotal: fiatTotal,
					Threshold: threshold,
					Owners:    owners,
				})
				wg.Done()
			}()
		}
		wg.Wait()

		return vault, nil
	})
}

//	{
//	   "threshold": 3,
//	   "owners": [
//	       {
//	           "value": "0xe01F8965b1eb362e00065f23D74dC694F1c4Cc5D",
//	           "name": null,
//	           "logoUri": null
//	       },
//	       {
//	           "value": "0x27B503fd677dd889ea3E7804DF492D2D809E6052",
//	           "name": null,
//	           "logoUri": null
//	       },
//	       {
//	           "value": "0x8C913aEc7443FE2018639133398955e0E17FB0C1",
//	           "name": null,
//	           "logoUri": null
//	       },
//	       {
//	           "value": "0x78f625A65Fc316D32d98d249b698fb509A6d98f2",
//	           "name": null,
//	           "logoUri": null
//	       },
//	       {
//	           "value": "0x9672c0e1639F159334Ca1288D4a24DEb02117291",
//	           "name": null,
//	           "logoUri": null
//	       }
//	   ]
//	}
type safeResponse struct {
	Threshold int   `json:"threshold"`
	Owners    []any `json:"owners"`
}

// https://safe-client.safe.global/v1/chains/1/safes/0x7FdA3253c94F09fE6950710E5273165283f8b283
func safe(chainId int, wallet string, client *http.Client) (int, int, error) {
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

//	{
//	   "fiatTotal": "689211.511"
//	}
type safeBalanceResponse struct {
	FiatTotal string `json:"fiatTotal"`
}

// https://safe-client.safe.global/v1/chains/1/safes/0x7FdA3253c94F09fE6950710E5273165283f8b283/balances/usd?trusted=true
func safeBalance(chainId int, wallet string, client *http.Client) (string, error) {
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
