// Package api is the desktop app's HTTP client for the platypus-server REST API.
//
// All requests inject a Bearer token (set via NewClient or FetchToken).
// The package returns raw response bytes for callers to decode; it does not
// dictate any particular JSON schema beyond the auth-token bootstrap.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// APIError is returned when the server responds with a non-2xx status.
// The raw body is preserved so callers can surface server-side error text.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("api: %d: %s", e.StatusCode, e.Body)
}

// Client is the HTTP client. Token is exported so the caller (Wails App)
// can persist it after FetchToken, but injection is handled here.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// NewClient constructs a Client. If token is empty, requests still go out
// (with no Authorization header) — useful for the FetchToken bootstrap.
func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Get issues GET path?query.
func (c *Client) Get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	return c.do(ctx, http.MethodGet, path, query, "", nil)
}

// Post issues POST path with a JSON-encoded body.
func (c *Client) Post(ctx context.Context, path string, body any) ([]byte, error) {
	encoded, err := encodeJSON(body)
	if err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}
	return c.do(ctx, http.MethodPost, path, nil, "application/json", encoded)
}

// Patch issues PATCH path with a JSON-encoded body.
func (c *Client) Patch(ctx context.Context, path string, body any) ([]byte, error) {
	encoded, err := encodeJSON(body)
	if err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}
	return c.do(ctx, http.MethodPatch, path, nil, "application/json", encoded)
}

// Delete issues DELETE path.
func (c *Client) Delete(ctx context.Context, path string) ([]byte, error) {
	return c.do(ctx, http.MethodDelete, path, nil, "", nil)
}

// PostRaw issues POST path with an arbitrary content-type and body.
// Used for binary uploads (file write).
func (c *Client) PostRaw(ctx context.Context, path, contentType string, body []byte) ([]byte, error) {
	return c.do(ctx, http.MethodPost, path, nil, contentType, body)
}

// FetchToken exchanges a server secret for a bearer token via
// POST /api/v1/auth/token. On success, c.Token is updated.
func (c *Client) FetchToken(ctx context.Context, secret string) error {
	body := map[string]string{"secret": secret}
	resp, err := c.Post(ctx, "/api/v1/auth/token", body)
	if err != nil {
		return err
	}
	var parsed struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return fmt.Errorf("decode token: %w", err)
	}
	if parsed.Token == "" {
		return errors.New("server returned empty token")
	}
	c.Token = parsed.Token
	return nil
}

func encodeJSON(body any) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	return json.Marshal(body)
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, contentType string, body []byte) ([]byte, error) {
	full := c.BaseURL + ensureLeadingSlash(path)
	if len(query) > 0 {
		full += "?" + query.Encode()
	}

	var rdr io.Reader
	if len(body) > 0 {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, full, rdr)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	return respBody, nil
}

func ensureLeadingSlash(p string) string {
	if p == "" || p[0] == '/' {
		return p
	}
	return "/" + p
}
