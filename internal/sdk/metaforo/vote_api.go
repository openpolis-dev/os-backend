package metaforo

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"
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
func CastVote(accessToken, groupName string, pollId int, options []int) error {
	apiPath := "/api/poll/vote"

	// prepare headers
	formHeader := AuthHeader(accessToken)

	// prepare json body
	payload := map[string]any{
		"poll_id":    pollId,
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
func RevokeVote(accessToken, groupName string, pollId int) error {
	apiPath := "/api/poll/remove"

	// prepare headers
	formHeader := AuthHeader(accessToken)

	// prepare json body
	payload := map[string]any{
		"poll_id": map[string]int{
			"poll_id": pollId,
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
