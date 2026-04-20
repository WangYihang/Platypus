package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	keyring "github.com/zalando/go-keyring"
)

// startTestServerWithNotify returns an httptest server that serves the
// /api/v1/auth/token endpoint (returning the given token) and a /notify
// WebSocket that sends `frames` once a client connects.
func startTestServerWithNotify(t *testing.T, token string, frames []string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"token":"` + token + `"}`))
	})
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("Accept: %v", err)
			return
		}
		defer c.CloseNow()
		for _, f := range frames {
			if err := c.Write(r.Context(), websocket.MessageText, []byte(f)); err != nil {
				return
			}
		}
		// hold the conn open until client disconnects
		<-r.Context().Done()
	})
	return httptest.NewServer(mux)
}

func TestApp_Connect_StartsNotifierAndForwardsEvents(t *testing.T) {
	keyring.MockInit()
	frames := []string{
		`{"Type":0,"Data":{"Client":{"hash":"abc"},"ServerHash":"srv1"}}`,
	}
	srv := startTestServerWithNotify(t, "tok", frames)
	defer srv.Close()

	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-notify-"+t.Name())

	var mu sync.Mutex
	var emitted []emitRecord
	a.emitFn = func(name string, data any) {
		mu.Lock()
		defer mu.Unlock()
		emitted = append(emitted, emitRecord{name: name, data: data})
	}

	a.AddProfile("p", srv.URL, "secret")
	if err := a.Connect("p"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer a.Disconnect()

	// wait up to 1s for the client_connected event (app:connection_changed
	// will also fire; we care about the notify: one here).
	deadline := time.After(1 * time.Second)
	found := false
	for !found {
		mu.Lock()
		for _, ev := range emitted {
			if ev.name == "notify:client_connected" {
				found = true
				break
			}
		}
		mu.Unlock()
		if found {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("no notify:client_connected event after 1s; got %v", emitted)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestApp_Disconnect_StopsNotifier(t *testing.T) {
	keyring.MockInit()
	srv := startTestServerWithNotify(t, "tok", nil)
	defer srv.Close()

	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-disconnect-"+t.Name())
	a.emitFn = func(string, any) {}
	a.AddProfile("p", srv.URL, "s")
	if err := a.Connect("p"); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if a.notifier == nil {
		t.Fatal("notifier should be set after Connect")
	}

	done := make(chan struct{})
	go func() {
		a.Disconnect()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Disconnect did not return within 2s")
	}
	if a.notifier != nil {
		t.Error("notifier should be nil after Disconnect")
	}
}

func TestApp_Connect_NotifierFailureDoesNotBlockConnect(t *testing.T) {
	keyring.MockInit()
	// Token endpoint succeeds; /notify endpoint refuses upgrades.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"tok"}`))
	})
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no upgrade", 400)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-notify-fail-"+t.Name())
	a.emitFn = func(string, any) {}
	a.AddProfile("p", srv.URL, "s")

	// Connect should still succeed (notifier failure is non-fatal — surfaced via UI later).
	if err := a.Connect("p"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !a.ConnectionStatus().Connected {
		t.Error("ConnectionStatus.Connected = false after successful Connect")
	}
	a.Disconnect()
}

type emitRecord struct {
	name string
	data any
}
