package metaforo

import (
	"bytes"
	"maps"
	"mime/multipart"
	"net/http"

	"github.com/rs/zerolog/log"
)

// GetComments list all comments for specified proposal in Metaforo
func GetComments(accessToken string, proposalId string) {

}

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
func AddComment(accessToken, content, proposalId, groupName string, replyId *string) error {
	apiPath := "/api/submit_post"

	// prepare headers
	formHeader := make(map[string]string)
	maps.Copy(formHeader, BaseHeader)
	formHeader["authorization"] = "Bearer " + accessToken

	// prepare multipart body
	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	_ = writer.WriteField("content", content)
	_ = writer.WriteField("sign", "")
	_ = writer.WriteField("signMsg", "")
	_ = writer.WriteField("login_type", "0")
	_ = writer.WriteField("thread_id", proposalId)
	_ = writer.WriteField("group_name", groupName)
	if replyId != nil {
		_ = writer.WriteField("reply_id", *replyId)
	}
	err := writer.Close()
	if err != nil {
		log.Error().Msgf("Prepare Multipart paramter error: %s", err)
		return err
	}

	// send request
	_, _, err = doHttpRequest[ApiResponseWrapper[any]](&httpRequestData{
		ApiUri:               apiBase + apiPath,
		HttpMethod:           http.MethodPost,
		MultipartBodyParams:  payload.Bytes(),
		MultipartContentType: writer.FormDataContentType(),
		Header:               formHeader,
	})

	return err
}
