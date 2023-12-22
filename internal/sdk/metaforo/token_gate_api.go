package metaforo

import (
	"bytes"
	"mime/multipart"
	"net/http"

	"github.com/rs/zerolog/log"
)

// CreateNftGate create nft gate in Metaforo
//
// `chainType`:
//
//	1: Ethereum
//	8: Polygon
//	7: BSC
//	9: Arbitrum
//
// `tokenType`:
//
//	0: ERC20
//	1: ERC721
//	2: ERC1155
func CreateNftGate(accessToken, groupName, chainType, tokenType, alias, address, tokenId string) error {
	apiPath := "/api/poll/setting"

	// prepare headers
	formHeader := AuthHeader(accessToken)

	// prepare multipart body
	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	_ = writer.WriteField("chain_type", chainType)
	_ = writer.WriteField("token_type", tokenType)
	_ = writer.WriteField("alias", alias)
	_ = writer.WriteField("address", address)
	_ = writer.WriteField("token_id", tokenId)
	_ = writer.WriteField("group_name", groupName)
	err := writer.Close()
	if err != nil {
		log.Error().Msgf("Prepare Multipart paramter error: %s", err)
		return err
	}

	// send request
	_, _, err = doHttpRequest[struct{}](&httpRequestData{
		ApiUri:               apiBase + apiPath,
		HttpMethod:           http.MethodPost,
		MultipartBodyParams:  payload.Bytes(),
		MultipartContentType: writer.FormDataContentType(),
		Header:               formHeader,
	})

	return err
}

func DeleteNftGate(accessToken string, groupName string, nftGateId string) error {
	apiPath := "/api/poll/delete"

	// prepare headers
	formHeader := AuthHeader(accessToken)

	// prepare multipart body
	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	_ = writer.WriteField("id", nftGateId)
	_ = writer.WriteField("group_name", groupName)
	err := writer.Close()
	if err != nil {
		log.Error().Msgf("Prepare Multipart paramter error: %s", err)
		return err
	}

	// send request
	_, _, err = doHttpRequest[struct{}](&httpRequestData{
		ApiUri:               apiBase + apiPath,
		HttpMethod:           http.MethodPost,
		MultipartBodyParams:  payload.Bytes(),
		MultipartContentType: writer.FormDataContentType(),
		Header:               formHeader,
	})

	return err
}
