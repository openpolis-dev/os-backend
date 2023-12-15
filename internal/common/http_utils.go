package common

import (
	"encoding/json"

	"github.com/rs/zerolog/log"
	"github.com/valyala/fasthttp"
)

// HttpRequestData struct saves all data used for http request
// TODO: Support query params with same name
type HttpRequestData struct {
	ApiUri      string
	HttpMethod  string
	QueryParams map[string]string
	BodyBytes   []byte
	Header      map[string]string
}

func DoHttpRequest[T any](requestData *HttpRequestData) (int, *T, error) {
	log.Debug().Msgf("Request data: %+v", requestData)
	// Prepare request and response
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

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

	if requestData.BodyBytes != nil {
		req.SetBody(requestData.BodyBytes)
	}

	log.Debug().Msgf("Request: %s", req.String())

	if err := fasthttp.Do(req, resp); err != nil {
		log.Error().Msgf("Send request error: %s", err)
	}

	var rsltData T
	err := json.Unmarshal(resp.Body(), &rsltData)
	if err != nil {
		log.Error().Msgf("Unmarshal error: %s", err)
		return 0, nil, err
	}

	return resp.StatusCode(), &rsltData, nil
}
