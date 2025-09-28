package metaforo

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/valyala/fasthttp"
)

const apiBase = "https://api.metaforo.io"

// HTTP client with 60 second timeout configuration
var httpClient = &fasthttp.Client{
	ReadTimeout:  60 * time.Second,
	WriteTimeout: 60 * time.Second,
	// Connection timeout for establishing new connections
	MaxConnDuration: 60 * time.Second,
	// Keep alive connections for better performance
	MaxIdleConnDuration: 90 * time.Second,
	// Maximum number of connections per host
	MaxConnsPerHost: 512,
}

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

// isTimeoutError checks if the given error is a timeout error
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "context deadline exceeded") ||
		strings.Contains(errStr, "i/o timeout")
}

func doHttpRequest[T any](requestData *httpRequestData) (int, *T, error) {
	return executeHttpRequest[T](requestData)
}

func executeHttpRequest[T any](requestData *httpRequestData) (int, *T, error) {
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

	if err := httpClient.Do(req, resp); err != nil {
		log.Error().Msgf("Request: %+v", req)
		log.Error().Msgf("Response: %+v", resp)
		log.Error().Msgf("Send request error: %s, req: %+v, resp: %+v", err, req, resp)

		// Check if the error is a timeout error
		if isTimeoutError(err) {
			return 0, nil, MetaforoTimeoutError
		}

		return 0, nil, err
	}

	// check status code
	if resp.StatusCode() != http.StatusOK {
		log.Error().Msgf("http request error, code: %d, request: %+v, resp: %+v", resp.StatusCode(), req, resp)
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
	// {"status":false,"code":40004,"description":"delete Tags Failed!","server":"master","data":{}}
	// {"status": false, "code": 40090, "description": "The token address is invalid!", "server": "master", "data": {} } // add gate token
	// {"status":false,"code":41002,"description":"Please login.","server":"master","data":{}}
	// {"status":false,"code":41004,"description":"sign error","server":"master","data":{}}
	// {"status": false, "code": 41108, "description": "The last one cannot be deleted", "server": "master", "data": {} } // delete category
	// TODO handle more failed situations
	if apiResp.Code == 20000 {
		return resp.StatusCode(), apiResp.Data, nil
	} else {
		log.Error().Msgf("http request error, code: %d, request: %+v, resp: %+v", resp.StatusCode(), req, resp)

		log.Error().Msgf("Request: %+v", req)
		log.Error().Msgf("Response: %+v", resp)

		if apiResp.Code == 40001 {
			return resp.StatusCode(), nil, GroupNotExist
		}
		if apiResp.Code == 40090 {
			return resp.StatusCode(), nil, TokenAddrInvalid
		}
		if apiResp.Code == 41002 {
			return resp.StatusCode(), nil, NoLogin
		}
		if apiResp.Code == 41004 {
			return resp.StatusCode(), nil, SignError
		}

		return resp.StatusCode(), nil, fmt.Errorf("metaforo error: %+v", apiResp.Description)
	}
}
