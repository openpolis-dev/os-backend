package metaforo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	seeauth "github.com/Taoist-Labs/see-auth-go"
	"github.com/Taoist-Labs/see-auth-go/proof"
	"github.com/Taoist-Labs/see-auth-go/signature"
)

// TokenResponse metaforo token api's response struct
//
//	{
//	   "wallet": "0xc1eE7cB74583D1509362467443C44f1FCa981283",
//	   "token": "24648|aq3kpWMHDQ88fXqkDdJiG54yVUqezSLLYgQl8Xhc",
//	   "user_id": 37398
//	}
//
//	{
//	   "status": false,
//	   "code": 40105,
//	   "description": "no user",
//	   "server": "master",
//	   "data": {}
//	}
type TokenResponse struct {
	Wallet string `json:"wallet"`
	Token  string `json:"token"`
	UserId int    `json:"user_id"`
}

// GetUserToken get metaforo's user token
// `userPrivateKey`: user wallet private key, no '0x' prefix
// `userWallet`: user wallet
// `seeAuthPrivateKey`: SeeDAO SeeAuth signer's private key, now is fixed to `59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d`
//
// for more, please see `TestToken` in `seeauth_metaforo_test.go`
func GetUserToken(userPrivateKey string, userWallet string, seeAuthPrivateKey string) (*TokenResponse, error) {
	// generate signature
	nonce := seeauth.GenerateNonce()
	message, sig, err := signature.Sign(nonce, 60*time.Second, userPrivateKey)
	if err != nil {
		return nil, err
	}

	// generating proof
	p, err := proof.Sign("0x0000000000000000000000000000000000000000", 60*time.Second, &proof.SchemaData{
		Wallet: userWallet,
		Vendor: "os+",
	}, seeAuthPrivateKey)
	if err != nil {
		return nil, err
	}

	payload := seeauth.SeeAuth{
		Wallet:     userWallet,
		WalletName: seeauth.WalletNameMetamask,
		Signature: &seeauth.Signature{
			Domain:    "app.seedao.xyz",
			Nonce:     nonce,
			Message:   message,
			Signature: sig,
		},
		Proof: &seeauth.Proof{
			Proof: p,
		},
	}

	// send request
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	//_, tokenResponse, err := doHttpRequest[TokenResponse](&httpRequestData{
	//	ApiUri:        "https://stage.metaforo.io/api/seeAuth?api_key=1",
	//	HttpMethod:    http.MethodPost,
	//	JsonBodyBytes: body,
	//})
	resp, err := http.Post("https://stage.metaforo.io/api/seeAuth?api_key=1", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed, status code: %d", resp.StatusCode)
	}

	var tokenResponse TokenResponse
	err = json.NewDecoder(resp.Body).Decode(&tokenResponse)
	if err != nil {
		return nil, err
	}

	// handle no-user error
	if tokenResponse.UserId == 0 {
		return nil, fmt.Errorf("no user")
	}

	return &tokenResponse, err
}
