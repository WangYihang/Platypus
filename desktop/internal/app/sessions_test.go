package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

// fixture: response from GET /api/client containing one TCPClient and one TermiteClient.
const clientListResponse = `{
  "status": true,
  "msg": {
    "h-tcp":     {"hash":"h-tcp","host":"1.2.3.4","port":3344,"alias":"t1","user":"root","os":"Linux","group_dispatch":false,"timestamp":"2026-04-20T10:00:00Z"},
    "h-termite": {"hash":"h-termite","host":"5.6.7.8","port":4455,"alias":"t2","user":"victim","os":"Linux","version":"1.5.1","group_dispatch":true,"timestamp":"2026-04-20T10:01:00Z"}
  }
}`

func TestApp_ListSessions_ParsesAndReturnsSlice(t *testing.T) {
	keyring.MockInit()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/token":
			w.Write([]byte(`{"token":"tok"}`))
		case "/api/client":
			if r.Header.Get("Authorization") != "Bearer tok" {
				t.Errorf("missing auth: %q", r.Header.Get("Authorization"))
			}
			w.Write([]byte(clientListResponse))
		case "/notify":
			// Notifier dials /notify on Connect; return 404 so the dial
			// fails cleanly (non-fatal — the REST path keeps working).
			w.WriteHeader(404)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-list-sessions-"+t.Name())
	a.AddProfile("p", srv.URL, "secret")
	if err := a.Connect("p"); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	got, err := a.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2", len(got))
	}
	// Sort by hash so test is stable.
	byHash := map[string]int{got[0].Hash: 0, got[1].Hash: 1}
	tcp := got[byHash["h-tcp"]]
	term := got[byHash["h-termite"]]

	if tcp.Encrypted {
		t.Error("h-tcp should be unencrypted")
	}
	if !term.Encrypted {
		t.Error("h-termite should be encrypted")
	}
	if tcp.Tag != "shell" {
		t.Errorf("h-tcp Tag = %q, want shell", tcp.Tag)
	}
	if term.Tag != "termite" {
		t.Errorf("h-termite Tag = %q, want termite", term.Tag)
	}
	if term.Version != "1.5.1" {
		t.Errorf("Version = %q", term.Version)
	}
}

func TestApp_ListSessions_NotConnected(t *testing.T) {
	keyring.MockInit()
	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-not-connected-"+t.Name())
	_, err := a.ListSessions()
	if err != ErrNotConnected {
		t.Errorf("err = %v, want ErrNotConnected", err)
	}
}

func TestApp_ListSessions_EmptyServerResponse(t *testing.T) {
	keyring.MockInit()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/token":
			w.Write([]byte(`{"token":"tok"}`))
		case "/api/client":
			w.Write([]byte(`{"status":true,"msg":{}}`))
		}
	}))
	defer srv.Close()

	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-empty-"+t.Name())
	a.AddProfile("p", srv.URL, "s")
	a.Connect("p")
	got, err := a.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestApp_ListSessions_ServerError(t *testing.T) {
	keyring.MockInit()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/token":
			w.Write([]byte(`{"token":"tok"}`))
		case "/api/client":
			w.WriteHeader(500)
			w.Write([]byte(`{"error":"boom"}`))
		}
	}))
	defer srv.Close()

	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-err-"+t.Name())
	a.AddProfile("p", srv.URL, "s")
	a.Connect("p")
	_, err := a.ListSessions()
	if err == nil {
		t.Fatal("expected error")
	}
}
