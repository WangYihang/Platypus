package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApp_UpgradeToTermite_HitsCorrectEndpoint(t *testing.T) {
	var got *http.Request
	var body []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"tok"}`))
	})
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/api/v1/sessions/", func(w http.ResponseWriter, r *http.Request) {
		got = r
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"status":true,"msg":"upgrade scheduled"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := freshConnectedApp(t, srv.URL)
	if err := a.UpgradeToTermite("plain-hash-123", "target-hash-abc"); err != nil {
		t.Fatalf("UpgradeToTermite: %v", err)
	}
	if got == nil {
		t.Fatal("server didn't see the request")
	}
	if got.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.Method)
	}
	wantPath := "/api/v1/sessions/plain-hash-123/upgrade"
	if !strings.HasSuffix(got.URL.Path, wantPath) {
		t.Errorf("path = %q, want suffix %q", got.URL.Path, wantPath)
	}
	var decoded struct {
		ListenerID string `json:"listener_id"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode body: %v — raw: %q", err, body)
	}
	if decoded.ListenerID != "target-hash-abc" {
		t.Errorf("listener_id = %q, want target-hash-abc", decoded.ListenerID)
	}
}

func TestApp_UpgradeToTermite_NotConnected(t *testing.T) {
	a := newTestApp(t)
	if err := a.UpgradeToTermite("a", "b"); err != ErrNotConnected {
		t.Errorf("err = %v", err)
	}
}
