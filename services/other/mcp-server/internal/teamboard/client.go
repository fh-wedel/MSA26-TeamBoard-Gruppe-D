// Package teamboard is a thin HTTP client for the TeamBoard gateway API,
// authenticated with a personal access token. It mirrors the request/response
// shapes used by frontend/src/api/client.ts — same gateway routes, same
// {"data": ...} / {"data": ..., "pagination": ...} envelope.
package teamboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Client calls the TeamBoard gateway with a Bearer personal access token.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// New creates a Client. baseURL is the gateway root (e.g. http://localhost),
// token is a raw "tbpat_..." personal access token from Settings.
func New(baseURL, token string) *Client {
	return &Client{baseURL: baseURL, token: token, httpClient: &http.Client{}}
}

type apiItem[T any] struct {
	Data T `json:"data"`
}

type apiList[T any] struct {
	Data       []T `json:"data"`
	Pagination struct {
		NextCursor *string `json:"next_cursor"`
	} `json:"pagination"`
}

type apiError struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Code   string `json:"code"`
}

func (c *Client) doItem(ctx context.Context, method, path string, body any, out any) error {
	respBody, err := c.do(ctx, method, path, body)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	wrapper := apiItem[json.RawMessage]{}
	if err := json.Unmarshal(respBody, &wrapper); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return json.Unmarshal(wrapper.Data, out)
}

// doList returns the decoded data slice and the next cursor (empty if none).
func (c *Client) doList(ctx context.Context, method, path string, out any) (string, error) {
	respBody, err := c.do(ctx, method, path, nil)
	if err != nil {
		return "", err
	}
	wrapper := apiList[json.RawMessage]{}
	if err := json.Unmarshal(respBody, &wrapper); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	raw, err := json.Marshal(wrapper.Data)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return "", err
	}
	cursor := ""
	if wrapper.Pagination.NextCursor != nil {
		cursor = *wrapper.Pagination.NextCursor
	}
	return cursor, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api/v1"+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call teamboard API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr apiError
		_ = json.Unmarshal(respBody, &apiErr)
		detail := apiErr.Detail
		if detail == "" {
			detail = apiErr.Title
		}
		return nil, fmt.Errorf("teamboard API %s %s: %d %s", method, path, resp.StatusCode, detail)
	}

	return respBody, nil
}
