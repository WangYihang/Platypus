package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/coder/websocket"
)

// Terminal opcodes mirror internal/api/websocket.go on the server side.
// Wire format is a single-byte opcode followed by payload bytes.
const (
	opcodeInput          byte = '0' // client → server
	opcodeOutput         byte = '0' // server → client
	opcodeResizeTerminal byte = '1' // client → server (RESIZE) / server → client (SET_WINDOW_TITLE)
	opcodeSetPreferences byte = '2' // server → client
)

// TerminalHandler receives server-pushed events for one terminal session.
// Implementations should be cheap — handlers run on the reader goroutine.
type TerminalHandler interface {
	OnOutput(data []byte)
	OnTitle(title string)
	OnPreferences(prefs string)
	OnClose(err error)
}

// Terminal is a single open PTY session over /ws/:hash.
type Terminal struct {
	conn    *websocket.Conn
	handler TerminalHandler

	cancel context.CancelFunc
	done   chan struct{}

	writeMu sync.Mutex
}

// DialTerminal opens a tty-subprotocol WebSocket to wsURL. The caller
// receives server frames via the handler until Close is called or the
// connection drops.
func DialTerminal(ctx context.Context, wsURL string, h TerminalHandler) (*Terminal, error) {
	if h == nil {
		return nil, errors.New("terminal: handler is nil")
	}
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		Subprotocols: []string{"tty"},
	})
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", wsURL, err)
	}

	innerCtx, cancel := context.WithCancel(ctx)
	t := &Terminal{
		conn:    conn,
		handler: h,
		cancel:  cancel,
		done:    make(chan struct{}),
	}
	go t.readLoop(innerCtx)
	return t, nil
}

func (t *Terminal) readLoop(ctx context.Context) {
	defer close(t.done)
	defer t.conn.CloseNow()

	var loopErr error
	defer func() {
		// OnClose must be called exactly once after the reader exits.
		t.handler.OnClose(loopErr)
	}()

	for {
		_, frame, err := t.conn.Read(ctx)
		if err != nil {
			loopErr = err
			return
		}
		if len(frame) == 0 {
			continue
		}
		opcode, payload := frame[0], frame[1:]
		switch opcode {
		case opcodeOutput:
			t.handler.OnOutput(payload)
		case opcodeResizeTerminal:
			// On the server→client direction this is SET_WINDOW_TITLE.
			t.handler.OnTitle(string(payload))
		case opcodeSetPreferences:
			t.handler.OnPreferences(string(payload))
		default:
			// Unknown opcode — silently skip.
		}
	}
}

// Write sends stdin bytes to the remote PTY.
func (t *Terminal) Write(data []byte) error {
	return t.send(opcodeInput, data)
}

// Resize notifies the remote PTY of new window dimensions. Only TermiteClient
// sessions honor this; plain reverse shells ignore it (server-side).
func (t *Terminal) Resize(cols, rows int) error {
	payload, err := json.Marshal(struct {
		Columns int `json:"Columns"`
		Rows    int `json:"Rows"`
	}{cols, rows})
	if err != nil {
		return err
	}
	return t.send(opcodeResizeTerminal, payload)
}

func (t *Terminal) send(opcode byte, payload []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if t.conn == nil {
		return errors.New("terminal: closed")
	}
	buf := make([]byte, 1+len(payload))
	buf[0] = opcode
	copy(buf[1:], payload)
	return t.conn.Write(context.Background(), websocket.MessageBinary, buf)
}

// Close shuts down the session. Safe to call multiple times.
func (t *Terminal) Close() {
	t.cancel()
	<-t.done
}

