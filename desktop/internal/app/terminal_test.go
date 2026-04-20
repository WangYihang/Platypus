package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	keyring "github.com/zalando/go-keyring"
)

// startConnectableServer returns an httptest.Server with /api/v1/auth/token
// + /notify (404 no-op) + /ws/<hash> that echoes the given behaviour.
func startConnectableServer(t *testing.T, wsBehaviour func(ctx context.Context, c *websocket.Conn)) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"tok"}`))
	})
	mux.HandleFunc("/api/v1/ws/ticket", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ticket":"test-ticket"}`))
	})
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})
	mux.HandleFunc("/ws/", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"tty"},
		})
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		wsBehaviour(ctx, c)
	})
	return httptest.NewServer(mux)
}

func TestApp_OpenTerminal_ForwardsOutputViaEmit(t *testing.T) {
	keyring.MockInit()
	srv := startConnectableServer(t, func(ctx context.Context, c *websocket.Conn) {
		c.Write(ctx, websocket.MessageBinary, []byte("0hello\n"))
		<-ctx.Done()
	})
	defer srv.Close()

	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-term-open-"+t.Name())
	var mu sync.Mutex
	var events []emitRecord
	a.emitFn = func(n string, d any) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, emitRecord{name: n, data: d})
	}
	a.AddProfile("p", srv.URL, "s")
	a.Connect("p")
	defer a.Disconnect()

	termID, err := a.OpenTerminal("abc")
	if err != nil {
		t.Fatalf("OpenTerminal: %v", err)
	}
	if termID == "" {
		t.Fatal("empty termID")
	}
	defer a.CloseTerminal(termID)

	deadline := time.After(1 * time.Second)
	for {
		mu.Lock()
		found := false
		for _, e := range events {
			if e.name == "terminal:output:"+termID {
				found = true
				break
			}
		}
		mu.Unlock()
		if found {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("no terminal output emitted; events=%v", events)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestApp_OpenTerminal_NotConnected(t *testing.T) {
	keyring.MockInit()
	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-term-nc-"+t.Name())
	_, err := a.OpenTerminal("abc")
	if err != ErrNotConnected {
		t.Errorf("err = %v, want ErrNotConnected", err)
	}
}

func TestApp_SendTerminalInput_And_Resize(t *testing.T) {
	keyring.MockInit()
	recv := make(chan []byte, 8)
	srv := startConnectableServer(t, func(ctx context.Context, c *websocket.Conn) {
		for {
			_, frame, err := c.Read(ctx)
			if err != nil {
				return
			}
			recv <- frame
		}
	})
	defer srv.Close()

	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-term-input-"+t.Name())
	a.emitFn = func(string, any) {}
	a.AddProfile("p", srv.URL, "s")
	a.Connect("p")
	defer a.Disconnect()

	termID, _ := a.OpenTerminal("abc")
	defer a.CloseTerminal(termID)

	if err := a.SendTerminalInput(termID, []byte("whoami\n")); err != nil {
		t.Fatalf("SendTerminalInput: %v", err)
	}
	if err := a.ResizeTerminal(termID, 120, 40); err != nil {
		t.Fatalf("ResizeTerminal: %v", err)
	}

	var sawInput, sawResize bool
	timeout := time.After(1 * time.Second)
	for !(sawInput && sawResize) {
		select {
		case f := <-recv:
			switch f[0] {
			case '0':
				if string(f[1:]) == "whoami\n" {
					sawInput = true
				}
			case '1':
				sawResize = true
			}
		case <-timeout:
			t.Fatalf("sawInput=%v sawResize=%v", sawInput, sawResize)
		}
	}
}

func TestApp_CloseTerminal_ReleasesSlot(t *testing.T) {
	keyring.MockInit()
	srv := startConnectableServer(t, func(ctx context.Context, c *websocket.Conn) { <-ctx.Done() })
	defer srv.Close()

	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-term-close-"+t.Name())
	a.emitFn = func(string, any) {}
	a.AddProfile("p", srv.URL, "s")
	a.Connect("p")
	defer a.Disconnect()

	termID, _ := a.OpenTerminal("abc")
	if err := a.CloseTerminal(termID); err != nil {
		t.Fatalf("CloseTerminal: %v", err)
	}
	// Closing again should be harmless.
	if err := a.CloseTerminal(termID); err == nil {
		t.Log("second Close allowed (idempotent)")
	}
	// Sending input after close should error.
	if err := a.SendTerminalInput(termID, []byte("x")); err == nil {
		t.Error("SendTerminalInput on closed terminal should error")
	}
}

func TestApp_Disconnect_ClosesAllTerminals(t *testing.T) {
	keyring.MockInit()
	srv := startConnectableServer(t, func(ctx context.Context, c *websocket.Conn) { <-ctx.Done() })
	defer srv.Close()

	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-term-disc-"+t.Name())
	a.emitFn = func(string, any) {}
	a.AddProfile("p", srv.URL, "s")
	a.Connect("p")

	id1, _ := a.OpenTerminal("a")
	id2, _ := a.OpenTerminal("b")
	if id1 == "" || id2 == "" || id1 == id2 {
		t.Fatalf("termIDs = %q, %q", id1, id2)
	}

	a.Disconnect()

	if err := a.SendTerminalInput(id1, []byte("x")); err == nil {
		t.Error("SendTerminalInput after Disconnect should error")
	}
}
