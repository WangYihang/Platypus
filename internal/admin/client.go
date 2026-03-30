// Package admin provides an HTTP client for interacting with the
// Platypus Server API. Used by platypus-admin CLI.
package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client is an HTTP client for the Platypus Server API.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// NewClient creates a new API client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Authenticate obtains a bearer token using the server secret.
func (c *Client) Authenticate(secret string) error {
	body, _ := json.Marshal(map[string]string{"secret": secret})
	resp, err := c.HTTPClient.Post(c.BaseURL+"/api/v1/auth/token", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("auth failed: status %d", resp.StatusCode)
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode auth response: %w", err)
	}
	c.Token = result.Token
	return nil
}

// Get sends an authenticated GET request.
func (c *Client) Get(path string, query url.Values) ([]byte, error) {
	u := c.BaseURL + path
	if query != nil {
		u += "?" + query.Encode()
	}
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	return c.do(req)
}

// Post sends an authenticated POST request with JSON body.
func (c *Client) Post(path string, body interface{}) ([]byte, error) {
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", c.BaseURL+path, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	return c.do(req)
}

// PostRaw sends an authenticated POST request with raw body.
func (c *Client) PostRaw(path string, body []byte) ([]byte, error) {
	req, _ := http.NewRequest("POST", c.BaseURL+path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/octet-stream")
	return c.do(req)
}

// Patch sends an authenticated PATCH request.
func (c *Client) Patch(path string, body interface{}) ([]byte, error) {
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("PATCH", c.BaseURL+path, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	return c.do(req)
}

// Delete sends an authenticated DELETE request.
func (c *Client) Delete(path string) ([]byte, error) {
	req, _ := http.NewRequest("DELETE", c.BaseURL+path, nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	return c.do(req)
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return data, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}
