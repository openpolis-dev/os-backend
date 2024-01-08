package metaforo

import (
	"bytes"
	"mime/multipart"
	"net/http"

	"github.com/rs/zerolog/log"
)

func Login(groupName, wallet, sign, signMsg, walletType string) (*LoginResponse, error) {
	apiPath := "/api/wallet/login"

	// prepare multipart body
	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	_ = writer.WriteField("web3_public_key", wallet)
	_ = writer.WriteField("sign", sign)
	_ = writer.WriteField("signMsg", signMsg)
	_ = writer.WriteField("wallet_type", walletType)
	_ = writer.WriteField("group_name", groupName)
	err := writer.Close()
	if err != nil {
		log.Error().Msgf("Prepare Multipart paramter error: %s", err)
		return nil, err
	}

	// send request
	_, resp, err := doHttpRequest[LoginResponse](&httpRequestData{
		ApiUri:               apiBase + apiPath,
		HttpMethod:           http.MethodPost,
		MultipartBodyParams:  payload.Bytes(),
		MultipartContentType: writer.FormDataContentType(),
		Header:               BaseHeader,
	})

	if err != nil {
		return nil, err
	}

	return resp, nil
}
