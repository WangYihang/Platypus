// Package llm wraps an OpenAI-compatible Chat Completions endpoint
// and a one-shot recording-summariser built on top.
//
// We deliberately speak OpenAI's HTTP shape (POST <base>/chat/
// completions, {"messages":[...], "model":"...", "max_tokens":N})
// rather than any vendor's bespoke API. Anthropic, OpenAI,
// OpenRouter, DeepSeek, local llama.cpp / vLLM, Together, Groq —
// all of them expose this shape. Operators pick the provider via
// PLATYPUS_LLM_BASE_URL and PLATYPUS_LLM_MODEL without recompiling.
//
// The package is intentionally tiny: no streaming, no tool use, no
// retries beyond a single context-bound timeout. The summariser is
// best-effort — it MUST never block the recording-finalise hot path
// nor surface as a user-visible error. Callers wrap calls in a
// goroutine + log on failure.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Default endpoint + model. Operators override via env vars (see
// FromEnv). The OpenAI defaults are picked because they're the
// canonical "OpenAI-style" — anyone who has any OpenAI-style key
// will succeed without configuration. Operators on Anthropic /
// OpenRouter etc. set base + model explicitly.
const (
	DefaultBaseURL = "https://api.openai.com/v1"
	DefaultModel   = "gpt-4o-mini"

	// 5 s is enough for a one-line summary on any sane provider —
	// gpt-4o-mini and Haiku both round-trip in ~1-2 s. Past 5 s we
	// give up rather than block the goroutine forever.
	DefaultTimeout = 5 * time.Second
)

// Client talks to one OpenAI-compatible endpoint with one model.
// Construct via New or FromEnv; Summarise is the only call site.
type Client struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

// New builds a Client. apiKey must be non-empty (callers should
// short-circuit when it is — there's no point dialling without
// auth). baseURL and model default to the OpenAI canonical values
// when blank.
func New(baseURL, apiKey, model string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if model == "" {
		model = DefaultModel
	}
	// Trim a trailing slash so the join below produces a clean URL
	// regardless of how the operator specifies the env var.
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		http:    &http.Client{Timeout: DefaultTimeout},
	}
}

// Available reports whether the client has the credentials it
// needs to make a call. Callers use this to skip the goroutine
// entirely when the operator hasn't configured an API key, which
// avoids logging a failure on every Finish.
func (c *Client) Available() bool {
	return c != nil && c.apiKey != ""
}

// Summarise asks the configured model for a one-line description
// of the given terminal-output text. The text should already be
// extracted + redacted; this layer doesn't know what's in it.
//
// Returns the trimmed summary on success. On any error (no
// credentials, HTTP failure, model error, malformed response,
// timeout) returns "" and a non-nil error. The caller logs once
// at WARN and persists NULL.
func (c *Client) Summarise(ctx context.Context, terminalText string) (string, error) {
	if !c.Available() {
		return "", errors.New("llm: API key not configured")
	}
	if strings.TrimSpace(terminalText) == "" {
		// Nothing to summarise — don't waste tokens.
		return "", nil
	}

	body, err := json.Marshal(chatRequest{
		Model:       c.model,
		MaxTokens:   80,
		Temperature: 0.2,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: terminalText},
		},
	})
	if err != nil {
		return "", fmt.Errorf("llm: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("llm: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 != 2 {
		// Read up to 512 bytes so we have something useful in the
		// log without dumping a multi-MB error page.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("llm: status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var parsed chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("llm: decode: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("llm: empty choices")
	}
	out := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if out == "" {
		return "", errors.New("llm: empty content")
	}
	return out, nil
}

// systemPrompt nudges the model to stay in the one-line shape we
// want for the card UI. Past tense + no first-person framing reads
// best in a list of mixed sessions.
const systemPrompt = "You summarise a terminal session for a fleet-management dashboard. " +
	"Reply with ONE sentence (≤25 words), past tense, focused on what the operator did " +
	"and any visible result. No markdown, no quotes, no \"the user\" / \"I\" framing."

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}
