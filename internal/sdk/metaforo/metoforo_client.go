package metaforo

import (
	"encoding/json"
	"maps"
	"net/http"

	"github.com/rs/zerolog/log"
	"github.com/valyala/fasthttp"
)

const apiBase = "https://api.metaforo.io"

var BaseHeader = map[string]string{
	"Accept":  "application/json",
	"api_key": "metaforo_website",
}

func AuthHeader(accessToken string) map[string]string {
	header := make(map[string]string)
	maps.Copy(header, BaseHeader)
	header["authorization"] = "Bearer " + accessToken
	return header
}

const JsonContentType = "application/json"
const FormContentType = "application/x-www-form-urlencoded"

type httpRequestData struct {
	ApiUri               string
	HttpMethod           string
	QueryParams          map[string]string
	JsonBodyBytes        []byte
	FormBodyParams       *fasthttp.Args
	MultipartBodyParams  []byte
	MultipartContentType string
	Header               map[string]string
}

func doHttpRequest[T any](requestData *httpRequestData) (int, *T, error) {
	// Prepare request
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	// Prepare response
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	// Build request object
	req.SetRequestURI(requestData.ApiUri)
	req.Header.SetMethod(requestData.HttpMethod)

	if requestData.Header != nil {
		for key, val := range requestData.Header {
			req.Header.Set(key, val)
		}
	}

	if requestData.QueryParams != nil {
		for key, val := range requestData.QueryParams {
			req.URI().QueryArgs().Add(key, val)
		}
	}

	if requestData.JsonBodyBytes != nil {
		req.SetBody(requestData.JsonBodyBytes)
		req.Header.SetContentType(JsonContentType)
	} else if requestData.FormBodyParams != nil {
		req.SetBody(requestData.FormBodyParams.QueryString())
		req.Header.SetContentType(FormContentType)
	} else if requestData.MultipartBodyParams != nil {
		req.SetBody(requestData.MultipartBodyParams)
		req.Header.SetContentType(requestData.MultipartContentType)
	}

	if err := fasthttp.Do(req, resp); err != nil {
		log.Error().Msgf("Send request error: %s", err)
	}

	// check status code
	if resp.StatusCode() != http.StatusOK {
		return resp.StatusCode(), nil, MetaforoError
	}

	// parse response
	var apiResp ApiResponseWrapper[T]
	err := json.Unmarshal(resp.Body(), &apiResp)
	if err != nil {
		log.Error().Msgf("Unmarshal error: %s", err)
		return resp.StatusCode(), nil, err
	}

	// check response code
	// { "status": true, "code": 20000, "description": "", "server": "rest", "data": { ... }}
	// {"status":false,"code":40001,"description":"Group not exist","server":"master","data":{}}
	// {"status": false, "code": 40011, "description": "INCARNA NFT is required to perform this action. Please check your NFT assets.", "server": "master", "data": {} }
	// {"status":false,"code":41002,"description":"Please login.","server":"master","data":{}}
	// TODO handle more failed situations
	if apiResp.Code == 20000 {
		return resp.StatusCode(), apiResp.Data, nil
	} else {
		if apiResp.Code == 40001 {
			return resp.StatusCode(), nil, GroupNotExist
		}
		if apiResp.Code == 40011 {
			return resp.StatusCode(), nil, NoVoteRight
		}
		if apiResp.Code == 41002 {
			return resp.StatusCode(), nil, NoLogin
		}

		return resp.StatusCode(), nil, MetaforoError
	}
}
