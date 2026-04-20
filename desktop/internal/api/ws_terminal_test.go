package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// startTerminalServer spins up a /ws/:hash endpoint. For each client, the
// goroutine runs `serverBehaviour` on the connection so each test can drive
// the opcodes it cares about.
func startTerminalServer(t *testing.T, serverBehaviour func(ctx context.Context, c *websocket.Conn)) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"tty"},
		})
		if err != nil {
			t.Errorf("Accept: %v", err)
			return
		}
		defer c.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		serverBehaviour(ctx, c)
	})
	return httptest.NewServer(mux)
}

type termHandler struct {
	mu       sync.Mutex
	outputs  [][]byte
	titles   []string
	prefs    []string
	closeErr error
	closed   bool
}

func (h *termHandler) OnOutput(data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.outputs = append(h.outputs, append([]byte(nil), data...))
}
func (h *termHandler) OnTitle(t string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.titles = append(h.titles, t)
}
func (h *termHandler) OnPreferences(p string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.prefs = append(h.prefs, p)
}
func (h *termHandler) OnClose(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	h.closeErr = err
}

func terminalURL(srvURL, hash string) string {
	return "ws" + strings.TrimPrefix(srvURL, "http") + "/ws/" + hash
}

func TestTerminal_DialAndReceiveOutput(t *testing.T) {
	srv := startTerminalServer(t, func(ctx context.Context, c *websocket.Conn) {
		// Server protocol: send SET_WINDOW_TITLE, SET_PREFERENCES, then an OUTPUT.
		frames := [][]byte{
			[]byte("1bash (ubuntu)"),
			[]byte("2{ }"),
			[]byte("0hello\n"),
		}
		for _, f := range frames {
			c.Write(ctx, websocket.MessageBinary, f)
		}
		<-ctx.Done()
	})
	defer srv.Close()

	h := &termHandler{}
	term, err := DialTerminal(context.Background(), terminalURL(srv.URL, "abc"), h)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer term.Close()

	// Poll up to 1s for the three frames to be delivered.
	deadline := time.After(1 * time.Second)
	for {
		h.mu.Lock()
		o, ti, pr := len(h.outputs), len(h.titles), len(h.prefs)
		h.mu.Unlock()
		if o >= 1 && ti >= 1 && pr >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timeout: outputs=%d titles=%d prefs=%d", o, ti, pr)
		case <-time.After(20 * time.Millisecond):
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if string(h.outputs[0]) != "hello\n" {
		t.Errorf("output[0] = %q", h.outputs[0])
	}
	if h.titles[0] != "bash (ubuntu)" {
		t.Errorf("title[0] = %q", h.titles[0])
	}
	if h.prefs[0] != "{ }" {
		t.Errorf("prefs[0] = %q", h.prefs[0])
	}
}

func TestTerminal_WriteSendsInputOpcode(t *testing.T) {
	recvCh := make(chan []byte, 4)
	srv := startTerminalServer(t, func(ctx context.Context, c *websocket.Conn) {
		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		recvCh <- data
		<-ctx.Done()
	})
	defer srv.Close()

	h := &termHandler{}
	term, err := DialTerminal(context.Background(), terminalURL(srv.URL, "abc"), h)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer term.Close()

	if err := term.Write([]byte("ls\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case got := <-recvCh:
		want := append([]byte{'0'}, []byte("ls\n")...)
		if string(got) != string(want) {
			t.Errorf("server got %q, want %q", got, want)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("server didn't receive input")
	}
}

func TestTerminal_ResizeSendsResizeOpcode(t *testing.T) {
	recvCh := make(chan []byte, 4)
	srv := startTerminalServer(t, func(ctx context.Context, c *websocket.Conn) {
		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		recvCh <- data
		<-ctx.Done()
	})
	defer srv.Close()

	h := &termHandler{}
	term, err := DialTerminal(context.Background(), terminalURL(srv.URL, "abc"), h)
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()

	if err := term.Resize(80, 24); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	select {
	case got := <-recvCh:
		if len(got) == 0 || got[0] != '1' {
			t.Fatalf("opcode byte = %v, want '1'", got[0])
		}
		var ws struct {
			Columns int
			Rows    int
		}
		if err := json.Unmarshal(got[1:], &ws); err != nil {
			t.Fatalf("resize payload not JSON: %v (%q)", err, got[1:])
		}
		if ws.Columns != 80 || ws.Rows != 24 {
			t.Errorf("resize payload = %+v", ws)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("server didn't receive resize")
	}
}

func TestTerminal_CloseTriggersOnClose(t *testing.T) {
	srv := startTerminalServer(t, func(ctx context.Context, c *websocket.Conn) {
		<-ctx.Done()
	})
	defer srv.Close()

	h := &termHandler{}
	term, err := DialTerminal(context.Background(), terminalURL(srv.URL, "abc"), h)
	if err != nil {
		t.Fatal(err)
	}

	term.Close()
	// Give the reader goroutine a moment to fire OnClose.
	deadline := time.After(1 * time.Second)
	for {
		h.mu.Lock()
		closed := h.closed
		h.mu.Unlock()
		if closed {
			return
		}
		select {
		case <-deadline:
			t.Fatal("OnClose never called")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestTerminal_DialFailure(t *testing.T) {
	h := &termHandler{}
	_, err := DialTerminal(context.Background(), "ws://127.0.0.1:1/ws/x", h)
	if err == nil {
		t.Fatal("expected dial error")
	}
}

func TestTerminal_UnknownOpcodeIgnored(t *testing.T) {
	srv := startTerminalServer(t, func(ctx context.Context, c *websocket.Conn) {
		// Unknown opcode '?' followed by known output.
		c.Write(ctx, websocket.MessageBinary, []byte("?wat"))
		c.Write(ctx, websocket.MessageBinary, []byte("0ok\n"))
		<-ctx.Done()
	})
	defer srv.Close()

	h := &termHandler{}
	term, err := DialTerminal(context.Background(), terminalURL(srv.URL, "abc"), h)
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()

	deadline := time.After(1 * time.Second)
	for {
		h.mu.Lock()
		o := len(h.outputs)
		h.mu.Unlock()
		if o >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("didn't receive output")
		case <-time.After(20 * time.Millisecond):
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.outputs) != 1 || string(h.outputs[0]) != "ok\n" {
		t.Errorf("outputs = %v", h.outputs)
	}
}
