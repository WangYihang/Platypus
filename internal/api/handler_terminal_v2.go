package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// Terminal WS opcodes — mirror what the browser's xterm.js adapter
// (desktop/frontend) speaks. Kept as bytes for zero-allocation
// prefix handling; the first byte of every frame is the opcode.
const (
	termOpcodeInput  byte = '0' // browser→server: stdin bytes
	termOpcodeResize byte = '1' // browser→server: JSON {columns, rows}
	termOpcodePause  byte = '2' // ignored (legacy zmodem)
	termOpcodeResume byte = '3' // ignored (legacy zmodem)
)

// termResize is the JSON shape the browser sends after the '1'
// opcode.
type termResize struct {
	Columns uint32 `json:"columns"`
	Rows    uint32 `json:"rows"`
}

// defaultShell is the command we open when the caller didn't
// specify one. Agents typically run as root with bash available;
// sh is the fallback.
const defaultShell = "/bin/bash"

// NewV2TerminalHandler returns the Gin handler for the v2 browser
// terminal endpoint. It bridges the xterm.js WS protocol to a
// STREAM_TYPE_PROCESS_OPEN stream on the named agent's live v2
// session.
func NewV2TerminalHandler(svc *core.AgentLinkService) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Param("agent_id")
		if agentID == "" {
			c.String(http.StatusBadRequest, "missing agent_id")
			return
		}
		sess, ok := svc.Get(agentID)
		if !ok {
			c.String(http.StatusNotFound, "agent %s not connected", agentID)
			return
		}

		ws, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
			Subprotocols: []string{"tty"},
			// Permissive since the REST origin is already authenticated.
			InsecureSkipVerify: true, //nolint:gosec // WebSocket Origin policy, not TLS
		})
		if err != nil {
			log.Warn("v2 terminal: ws upgrade for %s: %v", agentID, err)
			return
		}
		defer func() { _ = ws.CloseNow() }()

		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()

		if err := runV2Terminal(ctx, ws, sess, agentID); err != nil &&
			!errors.Is(err, context.Canceled) {
			log.Debug("v2 terminal %s: %v", agentID, err)
		}
	}
}

// runV2Terminal drives one browser WS session. Flow:
//
//  1. Read the first frame. It must be opcode '1' (resize) so we can
//     pass real dimensions when opening the process stream —
//     starting at 0×0 breaks ncurses apps (vim, tmux) until the
//     first SIGWINCH.
//  2. Open STREAM_TYPE_PROCESS_OPEN with those dimensions.
//  3. Read ProcessOpenResponse; abort on Error.
//  4. Run two pumps: WS → stream, stream → WS.
func runV2Terminal(ctx context.Context, ws *websocket.Conn, sess *link.Session, agentID string) error {
	firstType, firstData, err := ws.Read(ctx)
	if err != nil {
		return err
	}
	if firstType != websocket.MessageBinary || len(firstData) == 0 {
		return errors.New("first frame must be binary resize opcode")
	}
	cols, rows, err := parseResizeFrame(firstData)
	if err != nil {
		return err
	}

	req := &v2pb.ProcessOpenRequest{
		Command: defaultShell,
		Cols:    cols,
		Rows:    rows,
		Pty:     true,
	}
	meta, err := proto.Marshal(req)
	if err != nil {
		return err
	}
	stream, err := sess.Open(v2pb.StreamType_STREAM_TYPE_PROCESS_OPEN, meta, "terminal-"+agentID)
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()

	var ack v2pb.ProcessOpenResponse
	if err := link.ReadFrame(stream, &ack); err != nil {
		return err
	}
	if ack.Error != "" {
		return errors.New("agent open error: " + ack.Error)
	}

	// writeWS serialises the single browser-facing write stream.
	var wsMu sync.Mutex
	writeWS := func(op byte, payload []byte) error {
		wsMu.Lock()
		defer wsMu.Unlock()
		buf := make([]byte, 1+len(payload))
		buf[0] = op
		copy(buf[1:], payload)
		return ws.Write(ctx, websocket.MessageBinary, buf)
	}

	// WS → stream pump.
	done := make(chan struct{})
	var once sync.Once
	closeDone := func() { once.Do(func() { close(done) }) }

	go func() {
		defer closeDone()
		for {
			typ, data, err := ws.Read(ctx)
			if err != nil {
				return
			}
			if typ != websocket.MessageBinary || len(data) == 0 {
				continue
			}
			switch data[0] {
			case termOpcodeInput:
				if err := link.WriteFrame(stream, &v2pb.ProcessFrame{
					Payload: &v2pb.ProcessFrame_Stdin{Stdin: data[1:]},
				}); err != nil {
					return
				}
			case termOpcodeResize:
				cols, rows, perr := parseResizeFrame(data)
				if perr != nil {
					continue
				}
				_ = link.WriteFrame(stream, &v2pb.ProcessFrame{
					Payload: &v2pb.ProcessFrame_Resize{Resize: &v2pb.WindowSize{Cols: cols, Rows: rows}},
				})
			case termOpcodePause, termOpcodeResume:
				// ignored
			}
		}
	}()

	// stream → WS pump.
	for {
		var f v2pb.ProcessFrame
		if err := link.ReadFrame(stream, &f); err != nil {
			if errors.Is(err, io.EOF) {
				closeDone()
				return nil
			}
			closeDone()
			return err
		}
		switch p := f.Payload.(type) {
		case *v2pb.ProcessFrame_Stdout:
			if err := writeWS(termOpcodeInput, p.Stdout); err != nil {
				closeDone()
				return nil
			}
		case *v2pb.ProcessFrame_Stderr:
			if err := writeWS(termOpcodeInput, p.Stderr); err != nil {
				closeDone()
				return nil
			}
		case *v2pb.ProcessFrame_Exit:
			// Agent-side exit ends the session. Propagate an empty
			// input frame so xterm shows the usual "process exited"
			// beat and close the WS.
			closeDone()
			return nil
		}
	}
}

// parseResizeFrame unpacks the opcode-'1' JSON body into cols+rows.
// Accepts a frame with or without the leading opcode byte so callers
// that already stripped it still work.
func parseResizeFrame(data []byte) (cols, rows uint32, err error) {
	body := data
	if len(body) > 0 && body[0] == termOpcodeResize {
		body = body[1:]
	}
	var r termResize
	if err := json.Unmarshal(body, &r); err != nil {
		return 0, 0, err
	}
	if r.Columns == 0 {
		r.Columns = 80
	}
	if r.Rows == 0 {
		r.Rows = 24
	}
	return r.Columns, r.Rows, nil
}

// RegisterV2TerminalRoute mounts GET /api/v1/terminal/:agent_id/ws.
func RegisterV2TerminalRoute(engine *gin.Engine, svc *core.AgentLinkService) {
	engine.GET("/api/v1/terminal/:agent_id/ws", NewV2TerminalHandler(svc))
}
