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

func post(url, bearToken string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Notion-Version", "2022-06-28")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", bearToken))

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

type DatabaseData struct {
	Result []any `json:"results"`
}
