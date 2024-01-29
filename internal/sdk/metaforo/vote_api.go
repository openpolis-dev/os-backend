package metaforo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/valyala/fasthttp"
)

// CastVote vote a poll
//
// curl --location 'https://metaforo.io/api/poll/vote' \
// --header 'api_key: metaforo_website' \
// --header 'authorization: Bearer 21831|uLLroQDhdvk2OWKRHTP1wPR5vZX7vu1FmffgnBks' \
// --header 'Content-Type: application/json' \
//
//	--data '{
//		   "poll_id": 1731,
//		   "options": [
//		       4800
//		   ],
//		   "login_type": "0",
//		   "sign": "",
//		   "sign_msg": "",
//		   "group_name": "xs12"
//	}'
func CastVote(accessToken, groupName string, voteId int, options []int) error {
	apiPath := "/api/poll/vote"

	// prepare headers
	formHeader := AuthHeader(accessToken)

	// prepare json body
	payload := map[string]any{
		"poll_id":    voteId,
		"options":    options,
		"login_type": "0",
		"sign":       "",
		"sign_msg":   "",
		"group_name": groupName,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Error().Msgf("Prepare Json body error: %s", err)
		return err
	}

	// send request
	_, _, err = doHttpRequest[struct{}](&httpRequestData{
		ApiUri:        apiBase + apiPath,
		HttpMethod:    http.MethodPost,
		JsonBodyBytes: body,
		Header:        formHeader,
	})

	return err
}

// RevokeVote revoke a poll
//
// curl --location 'https://metaforo.io/api/poll/remove' \
// --header 'api_key: metaforo_website' \
// --header 'authorization: Bearer 21831|uLLroQDhdvk2OWKRHTP1wPR5vZX7vu1FmffgnBks' \
// --header 'Content-Type: application/json' \
//
//	--data '{
//	   "poll_id": {
//	       "poll_id": 1734
//	   },
//	   "group_name": "xs12"
//	}'
func RevokeVote(accessToken, groupName string, voteId int) error {
	apiPath := "/api/poll/remove"

	// prepare headers
	formHeader := AuthHeader(accessToken)

	// prepare json body
	payload := map[string]any{
		"poll_id": map[string]int{
			"poll_id": voteId,
		},
		"group_name": groupName,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Error().Msgf("Prepare Json body error: %s", err)
		return err
	}

	// send request
	_, _, err = doHttpRequest[struct{}](&httpRequestData{
		ApiUri:        apiBase + apiPath,
		HttpMethod:    http.MethodPost,
		JsonBodyBytes: body,
		Header:        formHeader,
	})

	return err
}

// UpdateVoteTime updates time of not started vote
// curl --location 'https://metaforo.io/api/poll/edit' \
// --header 'Authorization: Bearer 22453|uA3gXth....' \
// --form 'api_key="123"' \
// --form 'group_name="pic2"' \
// --form 'poll_id="1739"' \
// --form 'start_at="2024-06-02 09:40:21"' \
// --form 'close_at="2024-06-04 08:40:21"
func UpdateVoteTime(accessToken, groupName string, voteId int, startTs, endTs int64) error {
	apiPath := "/api/poll/edit"

	// prepare headers
	formHeader := AuthHeader(accessToken)

	formBody := fasthttp.Args{}
	formBody.Set("group_name", groupName)
	formBody.Set("poll_id", fmt.Sprintf("%d", voteId))
	// The start_at and close_at should be UTC timestamp
	formBody.Set("start_at", time.Unix(startTs, 0).UTC().Format(time.RFC3339))
	formBody.Set("close_at", time.Unix(endTs, 0).UTC().Format(time.RFC3339))

	// send request
	_, _, err := doHttpRequest[any](&httpRequestData{
		ApiUri:         apiBase + apiPath,
		HttpMethod:     http.MethodPost,
		FormBodyParams: &formBody,
		Header:         formHeader,
	})

	return err
}

// GetVoterList gets voter list of a poll
func GetVoterList(groupName string, OptionId int, page int) ([]*UserPollRecord, error) {
	apiPath := "/api/poll/list"
	formBody := fasthttp.Args{}
	formBody.Set("group_name", groupName)
	formBody.Set("option_id", fmt.Sprintf("%d", OptionId))
	formBody.Set("page", fmt.Sprintf("%d", page))

	// send request
	statusCode, body, err := doHttpRequest[UserPollRecordResponse](&httpRequestData{
		ApiUri:         apiBase + apiPath,
		HttpMethod:     http.MethodPost,
		FormBodyParams: &formBody,
		Header:         BaseHeader,
	})

	if statusCode != http.StatusOK || err != nil {
		log.Error().Msgf("create proposal request failed: %+v", err)
		return nil, err
	}

	return body.List, nil
}
