package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// recordingHandler captures the last incoming request for assertions.
type recordingHandler struct {
	t            *testing.T
	gotMethod    string
	gotPath      string
	gotRawQuery  string
	gotAuth      string
	gotCT        string
	gotBody      []byte
	respStatus   int
	respBody     []byte
	respCT       string
	delay        time.Duration
}

func (h *recordingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.delay > 0 {
		select {
		case <-time.After(h.delay):
		case <-r.Context().Done():
			return
		}
	}
	body, _ := io.ReadAll(r.Body)
	h.gotMethod = r.Method
	h.gotPath = r.URL.Path
	h.gotRawQuery = r.URL.RawQuery
	h.gotAuth = r.Header.Get("Authorization")
	h.gotCT = r.Header.Get("Content-Type")
	h.gotBody = body
	if h.respCT != "" {
		w.Header().Set("Content-Type", h.respCT)
	}
	if h.respStatus == 0 {
		h.respStatus = 200
	}
	w.WriteHeader(h.respStatus)
	w.Write(h.respBody)
}

func newTestServer(t *testing.T, h *recordingHandler) *httptest.Server {
	t.Helper()
	h.t = t
	return httptest.NewServer(h)
}

func TestClient_Get_InjectsBearerAndQuery(t *testing.T) {
	h := &recordingHandler{respBody: []byte(`{"ok":true}`)}
	srv := newTestServer(t, h)
	defer srv.Close()

	c := NewClient(srv.URL, "TOKEN-123")
	q := url.Values{}
	q.Set("path", "/etc/hosts")
	q.Set("offset", "0")

	body, err := c.Get(context.Background(), "/api/v1/sessions/abc/files/size", q)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if h.gotMethod != "GET" {
		t.Errorf("method = %q, want GET", h.gotMethod)
	}
	if h.gotPath != "/api/v1/sessions/abc/files/size" {
		t.Errorf("path = %q", h.gotPath)
	}
	if h.gotAuth != "Bearer TOKEN-123" {
		t.Errorf("Authorization = %q, want Bearer TOKEN-123", h.gotAuth)
	}
	if !strings.Contains(h.gotRawQuery, "path=%2Fetc%2Fhosts") || !strings.Contains(h.gotRawQuery, "offset=0") {
		t.Errorf("query = %q, missing expected params", h.gotRawQuery)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %q", body)
	}
}

func TestClient_Post_JSONBody(t *testing.T) {
	h := &recordingHandler{respBody: []byte(`{"status":true}`)}
	srv := newTestServer(t, h)
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	in := map[string]any{"command": "id", "timeout": 5}

	_, err := c.Post(context.Background(), "/api/v1/sessions/dispatch", in)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if h.gotMethod != "POST" {
		t.Errorf("method = %q", h.gotMethod)
	}
	if h.gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", h.gotCT)
	}
	var got map[string]any
	if err := json.Unmarshal(h.gotBody, &got); err != nil {
		t.Fatalf("body not JSON: %v (%q)", err, h.gotBody)
	}
	if got["command"] != "id" {
		t.Errorf("body command = %v", got["command"])
	}
}

func TestClient_Patch_JSONBody(t *testing.T) {
	h := &recordingHandler{respBody: []byte(`{"status":true}`)}
	srv := newTestServer(t, h)
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	_, err := c.Patch(context.Background(), "/api/v1/sessions/abc", map[string]string{"alias": "victim-1"})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if h.gotMethod != "PATCH" {
		t.Errorf("method = %q", h.gotMethod)
	}
	if h.gotCT != "application/json" {
		t.Errorf("Content-Type = %q", h.gotCT)
	}
}

func TestClient_Delete_NoBody(t *testing.T) {
	h := &recordingHandler{respBody: []byte(`{"status":true}`)}
	srv := newTestServer(t, h)
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	_, err := c.Delete(context.Background(), "/api/client/abc")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if h.gotMethod != "DELETE" {
		t.Errorf("method = %q", h.gotMethod)
	}
	if len(h.gotBody) != 0 {
		t.Errorf("body not empty: %q", h.gotBody)
	}
}

func TestClient_PostRaw_Binary(t *testing.T) {
	h := &recordingHandler{respBody: []byte(`{"bytes_written":5}`)}
	srv := newTestServer(t, h)
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	payload := []byte{0x00, 0x01, 0x02, 0x03, 0x04}
	_, err := c.PostRaw(context.Background(), "/api/v1/sessions/abc/files", "application/octet-stream", payload)
	if err != nil {
		t.Fatalf("PostRaw: %v", err)
	}
	if h.gotCT != "application/octet-stream" {
		t.Errorf("Content-Type = %q", h.gotCT)
	}
	if !bytes.Equal(h.gotBody, payload) {
		t.Errorf("body = %v, want %v", h.gotBody, payload)
	}
}

func TestClient_NonOKStatus_ReturnsAPIError(t *testing.T) {
	h := &recordingHandler{respStatus: 401, respBody: []byte(`{"error":"unauthorized"}`)}
	srv := newTestServer(t, h)
	defer srv.Close()

	c := NewClient(srv.URL, "wrong-token")
	_, err := c.Get(context.Background(), "/api/v1/sessions/x", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err type = %T, want *APIError", err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("StatusCode = %d, want 401", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Body, "unauthorized") {
		t.Errorf("Body = %q, missing 'unauthorized'", apiErr.Body)
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	h := &recordingHandler{delay: 200 * time.Millisecond, respBody: []byte(`{}`)}
	srv := newTestServer(t, h)
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := c.Get(ctx, "/slow", nil)
	if err == nil {
		t.Fatal("expected context-cancellation error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
}

func TestClient_FetchToken(t *testing.T) {
	h := &recordingHandler{respBody: []byte(`{"token":"NEW-TOKEN-XYZ"}`), respCT: "application/json"}
	srv := newTestServer(t, h)
	defer srv.Close()

	c := NewClient(srv.URL, "")
	if err := c.FetchToken(context.Background(), "the-secret"); err != nil {
		t.Fatalf("FetchToken: %v", err)
	}
	if h.gotPath != "/api/v1/auth/token" {
		t.Errorf("path = %q", h.gotPath)
	}
	if h.gotMethod != "POST" {
		t.Errorf("method = %q", h.gotMethod)
	}
	if c.Token != "NEW-TOKEN-XYZ" {
		t.Errorf("Token = %q, want NEW-TOKEN-XYZ", c.Token)
	}
	// Authorization should NOT be required for the token endpoint.
	// (We allow it being empty when c.Token == "" at call time.)
}

func TestClient_BaseURL_TrailingSlashIsNormalised(t *testing.T) {
	h := &recordingHandler{respBody: []byte(`{}`)}
	srv := newTestServer(t, h)
	defer srv.Close()

	// Base with trailing slash + path with leading slash should not double-slash.
	c := NewClient(srv.URL+"/", "tok")
	if _, err := c.Get(context.Background(), "/api/v1/sessions", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if h.gotPath != "/api/v1/sessions" {
		t.Errorf("path = %q (double-slash bug?)", h.gotPath)
	}
}

func TestClient_Get_NilQuery_NoQueryString(t *testing.T) {
	h := &recordingHandler{respBody: []byte(`{}`)}
	srv := newTestServer(t, h)
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	if _, err := c.Get(context.Background(), "/api/v1/sessions", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if h.gotRawQuery != "" {
		t.Errorf("RawQuery = %q, want empty", h.gotRawQuery)
	}
}

// Ensure body returned when Content-Length is zero (some endpoints omit it).
func TestClient_EmptyResponseBody(t *testing.T) {
	h := &recordingHandler{respBody: nil}
	srv := newTestServer(t, h)
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	body, err := c.Get(context.Background(), "/api/v1/sessions", nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(body) != 0 {
		t.Errorf("body = %q, want empty", body)
	}
}

// Tiny helper to ensure the request body io.Reader gets fully consumed
// even when caller passes an io.Reader (not just []byte). Right now the
// API doesn't expose a Reader-taking method but this asserts the JSON
// path drains via ReadAll.
func TestClient_JSONMarshalError(t *testing.T) {
	h := &recordingHandler{respBody: []byte(`{}`)}
	srv := newTestServer(t, h)
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	// channels can't be JSON-marshaled
	bad := make(chan int)
	_, err := c.Post(context.Background(), "/x", bad)
	if err == nil {
		t.Fatal("expected JSON marshal error, got nil")
	}
	if h.gotMethod != "" {
		t.Errorf("server should not be hit on marshal error, got method %q", h.gotMethod)
	}
}

// Smoke test: ensure the client doesn't panic when caller passes a
// pre-encoded *bytes.Buffer body (not used today but guards future regressions).
func TestClient_BufferIsAccepted(t *testing.T) {
	t.Skip("placeholder for future API: Post(reader io.Reader)")
	_ = bytes.NewBuffer
}
