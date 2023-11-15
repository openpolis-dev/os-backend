package sdk

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

func NotionDatabase(databaseId, bearToken string, body []byte) ([]byte, error) {
	return post(fmt.Sprintf("https://api.notion.com/v1/databases/%s/query", databaseId), bearToken, body)
}

func NotionPage(pageId, bearToken string) ([]byte, error) {
	return get(fmt.Sprintf("https://api.notion.com/v1/pages/%s", pageId), bearToken)
}

func NotionUser(userId, bearToken string) ([]byte, error) {
	return get(fmt.Sprintf("https://api.notion.com/v1/users/%s", userId), bearToken)
}

func post(url, bearToken string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Notion-Version", "2022-06-28")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", bearToken))

	return doHttp(req)
}

func get(url, bearToken string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Notion-Version", "2022-06-28")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", bearToken))

	return doHttp(req)
}

func doHttp(req *http.Request) ([]byte, error) {
	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("request Notion api failed")
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// NotionDatabaseData
//
//	{
//	   "object": "list",
//	   "results": [ ... ]
//	    "next_cursor": null,
//	    "has_more": false,
//	    "type": "page_or_database",
//	    "page_or_database": {},
//	    "developer_survey": "https://notionup.typeform.com/to/bllBsoI4?utm_source=postman",
//	    "request_id": "1d9814a5-ac49-4ae3-b702-300f5514f844"
//	}
type NotionDatabaseData struct {
	Result []any `json:"results"`
}

//// NotionPageData
////
////	{
////	   "object": "page",
////	   "id": "5ae6f630-58c0-4557-aecc-30850dec7abf",
////	   "created_time": "2023-10-12T02:58:00.000Z",
////	   "last_edited_time": "2023-10-14T04:28:00.000Z",
////	   "created_by": {
////	       "object": "user",
////	       "id": "e4e793cf-3e37-41c1-a38a-96c79164177c"
////	   },
////	   "last_edited_by": {
////	       "object": "user",
////	       "id": "35e1ae75-7dac-4497-9d90-46a7e48dbbf1"
////	   },
////	   "cover": {
////	       "type": "external",
////	       "external": {
////	           "url": "https://www.notion.so/images/page-cover/gradients_8.png"
////	       }
////	   },
////	   "icon": null,
////	   "parent": {
////	       "type": "database_id",
////	       "database_id": "73d83a0a-258d-4ac5-afa5-7a997114755a"
////	   },
////	   "archived": false,
////	   "properties": {
////	       "联络方式：微信": {
////	           "id": "%40kmc",
////	           "type": "rich_text",
////	           "rich_text": []
////	       }
////	   },
////	   "url": "https://www.notion.so/5ae6f63058c04557aecc30850dec7abf",
////	   "public_url": "https://seedao.notion.site/5ae6f63058c04557aecc30850dec7abf",
////	   "developer_survey": "https://notionup.typeform.com/to/bllBsoI4?utm_source=postman",
////	   "request_id": "5e99384c-771a-4006-8926-d6ea7c10d600"
////	}
//type NotionPageData struct {
//}

//// NotionUserData
////
////	{
////	   "object": "user",
////	   "id": "e4e793cf-3e37-41c1-a38a-96c79164177c",
////	   "name": "Tally Forms",
////	   "avatar_url": "https://s3-us-west-2.amazonaws.com/public.notion-static.com/03dd5720-9ef7-4d60-a537-c1d6fb20a77f/Zapier.png",
////	   "type": "bot",
////	   "bot": {},
////	   "developer_survey": "https://notionup.typeform.com/to/bllBsoI4?utm_source=postman",
////	   "request_id": "2972fa0e-6a40-4bc5-9052-905e1907a62a"
////	}
//type NotionUserData struct {
//}
