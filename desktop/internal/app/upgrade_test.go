package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApp_UpgradeToTermite_HitsCorrectEndpoint(t *testing.T) {
	var got *http.Request
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"tok"}`))
	})
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/api/client/", func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.Write([]byte(`{"status":true,"msg":"upgrade started"}`))
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
	if got.Method != "GET" {
		t.Errorf("method = %q", got.Method)
	}
	want := "/api/client/plain-hash-123/upgrade/target-hash-abc"
	if !strings.HasSuffix(got.URL.Path, want) {
		t.Errorf("path = %q, want suffix %q", got.URL.Path, want)
	}
}

func TestApp_UpgradeToTermite_NotConnected(t *testing.T) {
	a := newTestApp(t)
	if err := a.UpgradeToTermite("a", "b"); err != ErrNotConnected {
		t.Errorf("err = %v", err)
	}
}
