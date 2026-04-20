package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/coder/websocket"
)

// EventType mirrors the int values broadcast by the server's /notify
// WebSocket. See internal/core/server.go:WebSocketMessageType for the
// canonical source. We reproduce them here so the desktop app doesn't
// import server-internal packages.
type EventType int

const (
	EventClientConnected   EventType = 0
	EventClientDuplicated  EventType = 1
	EventServerDuplicated  EventType = 2
	EventCompiling         EventType = 3
	EventCompressing       EventType = 4
	EventUploading         EventType = 5
)

// Event is one notification frame. Data preserves the raw JSON so callers
// can unmarshal into a more specific shape per Type (Client connection,
// upgrade progress, etc.).
type Event struct {
	Type EventType
	Data json.RawMessage
}

// envelope matches the on-wire JSON shape: {"Type": <int>, "Data": {...}}.
type envelope struct {
	Type EventType       `json:"Type"`
	Data json.RawMessage `json:"Data"`
}

// EventHandler is invoked for every parsed event. It runs on the notifier's
// reader goroutine — handlers should not block.
type EventHandler func(Event)

// Notifier subscribes to the server's /notify WebSocket and dispatches
// each incoming frame to the registered handler. Lifetime is bounded by
// Start's ctx and explicit Stop().
type Notifier struct {
	url     string
	token   string
	handler EventHandler

	mu     sync.Mutex
	conn   *websocket.Conn
	cancel context.CancelFunc
	done   chan struct{}
}

// NewNotifier builds a Notifier. serverURL should be the full ws(s):// URL
// to the /notify endpoint (e.g. ws://127.0.0.1:7331/notify) OR the http(s)
// base URL of the server, in which case "/notify" is appended automatically.
// token is reserved for when the server adds Bearer auth on /notify.
func NewNotifier(serverURL, token string, h EventHandler) *Notifier {
	return &Notifier{
		url:     normaliseNotifyURL(serverURL),
		token:   token,
		handler: h,
	}
}

func normaliseNotifyURL(in string) string {
	switch {
	case strings.HasPrefix(in, "http://"):
		in = "ws://" + strings.TrimPrefix(in, "http://")
	case strings.HasPrefix(in, "https://"):
		in = "wss://" + strings.TrimPrefix(in, "https://")
	}
	if !strings.Contains(in[strings.Index(in, "//")+2:], "/") {
		in = strings.TrimRight(in, "/") + "/notify"
	}
	return in
}

// Start opens the WebSocket and spawns the reader goroutine. Returns an
// error if the dial fails. Use Stop() to terminate the goroutine cleanly.
// ctx scopes the connection lifetime; Stop() also cancels.
func (n *Notifier) Start(ctx context.Context) error {
	n.mu.Lock()
	if n.conn != nil {
		n.mu.Unlock()
		return errors.New("notifier: already started")
	}
	n.mu.Unlock()

	opts := &websocket.DialOptions{}
	if n.token != "" {
		opts.HTTPHeader = http.Header{"Authorization": []string{"Bearer " + n.token}}
	}
	conn, _, err := websocket.Dial(ctx, n.url, opts)
	if err != nil {
		return fmt.Errorf("dial %s: %w", n.url, err)
	}

	innerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	n.mu.Lock()
	n.conn = conn
	n.cancel = cancel
	n.done = done
	n.mu.Unlock()

	go n.readLoop(innerCtx, conn, done)
	return nil
}

func (n *Notifier) readLoop(ctx context.Context, conn *websocket.Conn, done chan struct{}) {
	// LIFO: CloseNow runs first (frees the fd), close(done) runs last.
	// CloseNow skips the WS close handshake so it doesn't block on a server
	// that's slow to ACK — important when Stop() forces shutdown.
	defer close(done)
	defer conn.CloseNow()

	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			// Malformed frame — skip silently. (Future: surface via a
			// separate error channel for telemetry.)
			continue
		}
		n.handler(Event{Type: env.Type, Data: env.Data})
	}
}

// Stop cancels the context and waits for the reader goroutine to exit.
// Safe to call multiple times.
func (n *Notifier) Stop() {
	n.mu.Lock()
	cancel := n.cancel
	done := n.done
	n.conn = nil
	n.cancel = nil
	n.done = nil
	n.mu.Unlock()

	if cancel == nil {
		return
	}
	cancel()
	if done != nil {
		<-done
	}
}
