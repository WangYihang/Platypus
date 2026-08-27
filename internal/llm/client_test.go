package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_Summarise_HappyPath(t *testing.T) {
	var seenReq chatRequest
	var seenAuth string
	var seenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &seenReq)
		_ = json.NewEncoder(w).Encode(chatResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{
				{Message: chatMessage{Role: "assistant", Content: "  Updated nginx config and reloaded.  "}},
			},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key", "fake-model")
	out, err := c.Summarise(context.Background(), "ls\nedited /etc/nginx\nreload\n")
	if err != nil {
		t.Fatalf("Summarise: %v", err)
	}
	if out != "Updated nginx config and reloaded." {
		t.Errorf("trimmed output mismatch: %q", out)
	}
	if seenPath != "/chat/completions" {
		t.Errorf("path = %q", seenPath)
	}
	if seenAuth != "Bearer test-key" {
		t.Errorf("auth = %q", seenAuth)
	}
	if seenReq.Model != "fake-model" {
		t.Errorf("model = %q", seenReq.Model)
	}
	if len(seenReq.Messages) != 2 || seenReq.Messages[0].Role != "system" {
		t.Errorf("messages = %+v", seenReq.Messages)
	}
}

func TestClient_Summarise_NoCredentials(t *testing.T) {
	c := New("https://example.com", "", "")
	_, err := c.Summarise(context.Background(), "anything")
	if err == nil || !strings.Contains(err.Error(), "API key") {
		t.Errorf("err = %v", err)
	}
	if c.Available() {
		t.Errorf("Available should be false without an API key")
	}
}

func TestClient_Summarise_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"rate limit"}`, http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "m")
	_, err := c.Summarise(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Errorf("err = %v", err)
	}
}

func TestClient_Summarise_EmptyInput(t *testing.T) {
	// Should NOT make an HTTP call at all — returns empty +
	// nil error so callers can short-circuit.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server hit despite empty input")
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "m")
	out, err := c.Summarise(context.Background(), "   \n\n  ")
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if out != "" {
		t.Errorf("out = %q, want empty", out)
	}
}

func TestClient_Summarise_Timeout(t *testing.T) {
	// Deliberately stall the server past the request's deadline.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "m")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.Summarise(ctx, "x")
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}
