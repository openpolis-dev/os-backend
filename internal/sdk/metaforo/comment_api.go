package metaforo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"

	"github.com/rs/zerolog/log"
)

// AddComment add/reply comment to specified proposal in Metaforo
// add or reply comment `content` to proposal `proposalId` in group `groupName`, if `replyId` is not nil, then reply to comment `replyId`.
//
// curl --location 'https://metaforo.io/api/submit_post' \
// --header 'api_key: metaforo_website' \
// --header 'authorization: Bearer 21831|uLLroQDhdvk2OWKRHTP1wPR5vZX7vu1FmffgnBks' \
// --form 'content="[{\"insert\":\"测试\n\"}]"' \
// --form 'sign=""' \
// --form 'signMsg=""' \
// --form 'login_type="0"' \
// --form 'reply_id="1964128"' \
// --form 'thread_id="47945"' \
// --form 'group_name="testttt"'
func AddComment(accessToken, groupName string, proposalId int, content []*NewContentRequest, replyId *string) error {
	apiPath := "/api/submit_post"

	// prepare headers
	formHeader := AuthHeader(accessToken)

	// prepare multipart body
	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	_ = writer.WriteField("sign", "")
	_ = writer.WriteField("signMsg", "")
	_ = writer.WriteField("login_type", "0")
	_ = writer.WriteField("thread_id", fmt.Sprintf("%d", proposalId))
	_ = writer.WriteField("group_name", groupName)
	if content != nil {
		c, _ := json.Marshal(content)
		_ = writer.WriteField("content", string(c))
	}
	if replyId != nil {
		_ = writer.WriteField("reply_id", *replyId)
	}
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

func EditComment(accessToken, groupName, postId string, content []*NewContentRequest) error {
	apiPath := "/api/edit_post"

	// prepare headers
	formHeader := AuthHeader(accessToken)

	// prepare multipart body
	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	_ = writer.WriteField("sign", "")
	_ = writer.WriteField("signMsg", "")
	_ = writer.WriteField("post_id", postId)
	_ = writer.WriteField("group_name", groupName)
	if content != nil {
		c, _ := json.Marshal(content)
		_ = writer.WriteField("content", string(c))
	}
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

func DeleteComment(accessToken, groupName, postId string) error {
	apiPath := "/api/delete_post"

	// prepare headers
	formHeader := AuthHeader(accessToken)

	// prepare multipart body
	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	_ = writer.WriteField("post_id", postId)
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
