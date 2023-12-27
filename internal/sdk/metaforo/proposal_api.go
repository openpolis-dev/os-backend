package metaforo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"

	"github.com/gomarkdown/markdown"
	"github.com/microcosm-cc/bluemonday"
	"github.com/rs/zerolog/log"
)

// GetProposals get specified proposal in Metaforo.
//
// curl --location 'https://metaforo.io/api/get_thread/47967?group_name=testttt' \
// --header 'api_key: metaforo_website' \
// --header 'authorization: Bearer 21831|uLLroQDhdvk2OWKRHTP1wPR5vZX7vu1FmffgnBks'
func GetProposals(proposalId, groupName string) (*Thread, error) {
	apiPath := fmt.Sprintf("/api/get_thread/%s", proposalId)

	_, resp, err := doHttpRequest[ProposalResponse](&httpRequestData{
		ApiUri:      apiBase + apiPath,
		HttpMethod:  http.MethodGet,
		QueryParams: map[string]string{"group_name": groupName},
		Header:      BaseHeader,
	})
	if err != nil {
		return nil, err
	}

	return resp.Thread, nil
}

// ListProposals list all proposals in Metaforo.
// `group_name` parameter located at `paginationParams` parameter.
// `tag_id=0` means get all proposals not one tag.
//
// curl --location 'https://metaforo.io/api/thread/list?page=1&per_page=10&filter=all&category_index_id=0&tag_id=0&sort=new&group_name=testttt' \
// --header 'api_key: metaforo_website' \
// --header 'authorization: Bearer 21831|uLLroQDhdvk2OWKRHTP1wPR5vZX7vu1FmffgnBks'
func ListProposals(paginationParams *PaginationParams) ([]*Thread, error) {
	apiPath := "/api/thread/list"

	_, resp, err := doHttpRequest[ProposalListResponse](&httpRequestData{
		ApiUri:      apiBase + apiPath,
		HttpMethod:  http.MethodGet,
		QueryParams: paginationParams.ToMap(),
		Header:      BaseHeader,
	})
	if err != nil {
		return nil, err
	}

	return resp.Threads, nil
}

// CreateProposal create new proposal in Metaforo.
//
//	{
//	   "status": true,
//	   "code": 20000,
//	   "description": "",
//	   "server": "rest",
//	   "data": {
//	       "group": {
//	           "id": 10462,
//	           "name": "xs12",
//	           "title": "xs12",
//	           "poll_setting": [
//	               {
//	                   "id": 63,
//	                   "group_id": 10462,
//	                   "chain_type": 1,
//	                   "token_type": 0,
//	                   "address": "0xdac17f958d2ee523a2206206994597c13d831ec7",
//	                   "token_id": 0,
//	                   "alias": "ERC20",
//	               },
//	               {
//	                   "id": 64,
//	                   "group_id": 10462,
//	                   "chain_type": 1,
//	                   "token_type": 1,
//	                   "address": "0xfdf5acd92840e796955736b1bb9cc832740744ba",
//	                   "token_id": 0,
//	                   "alias": "ERC721",
//	               },
//	               {
//	                   "id": 65,
//	                   "group_id": 10462,
//	                   "chain_type": 1,
//	                   "token_type": 2,
//	                   "address": "0x6811f2f20c42f42656a3c8623ad5e9461b83f719",
//	                   "token_id": 100200402,
//	                   "alias": "ERC1155",
//	               }
//	           ],
//	           "tags": [
//	               {
//	                   "name": "投票中",
//	                   "color": "",
//	                   "thread_count": 0,
//	                   "id": 472,
//	                   "order": 999,
//	                   "type": 0
//	               },
//	               {
//	                   "name": "待审核",
//	                   "color": "",
//	                   "thread_count": 0,
//	                   "id": 471,
//	                   "order": 999,
//	                   "type": 0
//	               }
//	           ],
//	           "categories": [
//	               {
//	                   "category_id": 1,
//	                   "group_id": 10462,
//	                   "name": "General Discussions",
//	                   "parent_id": 0,
//	                   "type": 0,
//	                   "order": null,
//	                   "thread_count": 0,
//	                   "template_id": 0,
//	                   "post_count": 0,
//	                   "icon_unicode": "",
//	                   "can_see": 1,
//	                   "can_create": 1,
//	                   "children": []
//	               }
//	           ]
//	       }
//	   }
//	}
func CreateProposal(accessToken, groupName, categoryIndexId, title string, content string, tags []*NewProposalTagRequest, voteFormData string) (*ProposalResponse, error) {
	apiPath := "/api/submit_thread"

	// prepare headers
	formHeader := AuthHeader(accessToken)

	bodyBytes, contentType, err := createCreateOrUpdateFormData(groupName, title, categoryIndexId, content, tags, voteFormData, 0)
	if err != nil {
		log.Error().Msgf("Failed to create proposal form data: %+v", err)
		return nil, err
	}

	// send request
	statusCode, resp, err := doHttpRequest[ProposalResponse](&httpRequestData{
		ApiUri:               apiBase + apiPath,
		HttpMethod:           http.MethodPost,
		MultipartBodyParams:  bodyBytes,
		MultipartContentType: contentType,
		Header:               formHeader,
	})

	if statusCode != http.StatusOK || err != nil {
		log.Error().Msgf("create proposal request failed: %+v", err)
		return nil, err
	}

	return resp, nil
}

func UpdateProposal(accessToken, groupName, categoryIndexId, title string, content string, tags []*NewProposalTagRequest, voteFormData string, threadId int) (*ProposalResponse, error) {
	apiPath := "/api/edit_post"

	// prepare headers
	formHeader := AuthHeader(accessToken)

	bodyBytes, contentType, err := createCreateOrUpdateFormData(groupName, title, categoryIndexId, content, tags, voteFormData, threadId)
	if err != nil {
		log.Error().Msgf("Failed to create proposal form data: %+v", err)
		return nil, err
	}

	// send request
	statusCode, resp, err := doHttpRequest[ProposalResponse](&httpRequestData{
		ApiUri:               apiBase + apiPath,
		HttpMethod:           http.MethodPost,
		MultipartBodyParams:  bodyBytes,
		MultipartContentType: contentType,
		Header:               formHeader,
	})

	if statusCode != http.StatusOK || err != nil {
		log.Error().Msgf("create proposal request failed: %+v", err)
		return nil, err
	}

	return resp, nil
}

// DeleteProposal delete specified proposal in Metaforo.
// please note `postId` is not proposal id
//
// curl --location 'https://metaforo.io/api/delete_post' \
// --header 'api_key: metaforo_website' \
// --header 'authorization: Bearer 21831|uLLroQDhdvk2OWKRHTP1wPR5vZX7vu1FmffgnBks' \
// --form 'post_id="1964155"' \
// --form 'group_name="xs12"'
func DeleteProposal(accessToken, postId, groupName string) error {
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
	_, _, err = doHttpRequest[any](&httpRequestData{
		ApiUri:               apiBase + apiPath,
		HttpMethod:           http.MethodPost,
		MultipartBodyParams:  payload.Bytes(),
		MultipartContentType: writer.FormDataContentType(),
		Header:               formHeader,
	})

	return err
}

func createCreateOrUpdateFormData(groupName, title, categoryIndexId, content string, tags []*NewProposalTagRequest, voteFormData string, threadId int) ([]byte, string, error) {
	// prepare multipart body
	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	_ = writer.WriteField("sign", "")
	_ = writer.WriteField("signMsg", "")
	_ = writer.WriteField("title", title)
	_ = writer.WriteField("category_index_id", categoryIndexId)
	_ = writer.WriteField("login_type", "0")
	_ = writer.WriteField("group_name", groupName)

	if threadId != 0 {
		_ = writer.WriteField("thread_id", fmt.Sprintf("%d", threadId))
	}

	// Set editor_type to 1 to support markdown, and for Markdown should be passed
	_ = writer.WriteField("editor_type", "1")
	_ = writer.WriteField("content", content)

	maybeUnsafeHTML := markdown.ToHTML([]byte(content), nil, nil)
	html := bluemonday.UGCPolicy().SanitizeBytes(maybeUnsafeHTML)
	_ = writer.WriteField("html", string(html))

	if tags != nil {
		t, _ := json.Marshal(tags)
		_ = writer.WriteField("tags", string(t))
	}

	if voteFormData != "" {
		_ = writer.WriteField("polls", string(voteFormData))
	}

	err := writer.Close()
	if err != nil {
		log.Error().Msgf("Prepare Multipart paramter error: %+v", err)
		return nil, "", err
	}

	return payload.Bytes(), writer.FormDataContentType(), nil
}
