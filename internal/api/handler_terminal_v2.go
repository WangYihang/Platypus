package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/recording"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
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
// session. When recMgr is non-nil and enabled, the handler also
// streams every stdout/stderr/resize event into an asciinema v2 cast
// file via the recording.Manager so operators can replay the session
// later.
func NewV2TerminalHandler(svc *core.AgentLinkService, recMgr *recording.Manager) gin.HandlerFunc {
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

		// Origin policy lives in wsAcceptOptions: strict same-origin in
		// production, InsecureSkipVerify only when PLATYPUS_DEV=1
		// (e.g. Vite-dev backend / SPA on different ports). See L1 in
		// the security audit and ws_origin_test.go for the contract.
		ws, err := websocket.Accept(c.Writer, c.Request, wsAcceptOptions("tty"))
		if err != nil {
			log.Warn("v2 terminal: ws upgrade for %s: %v", agentID, err)
			return
		}
		defer func() { _ = ws.CloseNow() }()

		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()

		// Audit "shell open" up-front so an attempt that drops mid-way
		// (browser closed before the first frame, agent rejected the
		// PTY open, …) still leaves a forensic trail. The matching
		// "shell close" row below carries the wallclock duration so
		// per-session lifetime queries on the activities table work
		// without joining transfer-style records.
		start := time.Now().UTC()
		RecordActivity(c, ActivityInput{
			Category:   storage.CategorySession,
			Action:     "shell.open",
			TargetType: "agent",
			TargetID:   agentID,
			At:         start,
		})

		runErr := runV2Terminal(ctx, ws, sess, agentID, recMgr, recordingMetaFromContext(c, agentID, start))
		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			log.Debug("v2 terminal %s: %v", agentID, runErr)
		}

		closeOutcome := storage.OutcomeSuccess
		var closeErr string
		if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, io.EOF) {
			closeOutcome = storage.OutcomeError
			closeErr = runErr.Error()
		}
		dur := time.Since(start).Milliseconds()
		RecordActivity(c, ActivityInput{
			Category:   storage.CategorySession,
			Action:     "shell.close",
			TargetType: "agent",
			TargetID:   agentID,
			Outcome:    closeOutcome,
			Error:      closeErr,
			DurationMs: &dur,
			At:         time.Now().UTC(),
		})
	}
}

// recordingMetaFromContext bundles the project / host / user
// attribution we know up-front so runV2Terminal can call
// recording.Manager.Begin without re-reading the gin context (which
// the WS goroutine has lost access to by the time the recorder needs
// it).
func recordingMetaFromContext(c *gin.Context, agentID string, start time.Time) recording.BeginInput {
	in := recording.BeginInput{
		AgentID:   agentID,
		ProjectID: c.Param("pid"),
		Shell:     defaultShell,
		StartedAt: start,
	}
	if p, ok := PrincipalFromContext(c); ok {
		in.UserID = p.UserID
	}
	return in
}

// runV2Terminal drives one browser WS session. Flow:
//
//  1. Read the first frame. It must be opcode '1' (resize) so we can
//     pass real dimensions when opening the process stream —
//     starting at 0×0 breaks ncurses apps (vim, tmux) until the
//     first SIGWINCH.
//  2. Open STREAM_TYPE_PROCESS_OPEN with those dimensions.
//  3. Read ProcessOpenResponse; abort on Error.
//  4. Open a recording session (no-op when recMgr is disabled).
//  5. Run two pumps: WS → stream, stream → WS.
func runV2Terminal(ctx context.Context, ws *websocket.Conn, sess *link.Session, agentID string, recMgr *recording.Manager, recMeta recording.BeginInput) error {
	// Child context so closeDone (below) can cancel the WS-read pump
	// when the stream-side pump is the one that exits first. The
	// parent ctx still propagates cancellation in the other direction.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

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

	// Resolve host_id for this agent so the recording row is filterable
	// per-host on the UI list page. Best-effort: a missing host is
	// surprising (the RBAC chain already enforced project membership)
	// but we still record the session under the agent_id.
	if recMgr != nil && recMgr.Enabled() && recMeta.HostID == "" {
		if hid, ok := lookupHostIDForAgent(ctx, agentID); ok {
			recMeta.HostID = hid
		}
	}
	recMeta.Cols = cols
	recMeta.Rows = rows

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

	// Begin recording AFTER the agent acknowledges the PTY open — a
	// rejected open should not leave an empty .cast on disk. When the
	// manager is disabled Begin returns a no-op session.
	var recSess *recording.Session
	if recMgr != nil {
		// Use a context detached from the request so the row finalisation
		// can run after the WS has closed.
		bgCtx := context.Background()
		var beginErr error
		recSess, beginErr = recMgr.Begin(bgCtx, recMeta)
		if beginErr != nil {
			log.L.Warn("recording_begin_failed",
				"agent_id", agentID,
				"error", beginErr.Error(),
			)
		}
	}
	defer func() {
		if recSess != nil {
			recSess.Finish(context.Background(), "")
		}
	}()

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

	// WS → stream pump. closeDone tears down both pumps: closing the
	// stream unblocks link.ReadFrame on the main pump (so a silent
	// browser disconnect can no longer leak the goroutine + the
	// agent-side PTY); cancelling ctx unblocks ws.Read on this pump
	// when the main pump is the one that exits first.
	done := make(chan struct{})
	var once sync.Once
	closeDone := func() {
		once.Do(func() {
			close(done)
			_ = stream.Close()
			cancel()
		})
	}

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
				recSess.WriteInput(data[1:])
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
				recSess.WriteResize(cols, rows)
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
			recSess.WriteOutput(p.Stdout)
			if err := writeWS(termOpcodeInput, p.Stdout); err != nil {
				closeDone()
				return nil
			}
		case *v2pb.ProcessFrame_Stderr:
			recSess.WriteOutput(p.Stderr)
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

// Sub-MIN dims indicate a layout transient on the browser side
// (xterm-fit reading from a drawer/panel that's still animating).
// Forwarding them to the agent's PTY thrashes the live shell AND
// pollutes the recorded .cast with "huge text" jumps mid-stream.
// The browser already filters this; the server clamp is defense in
// depth so an old / third-party client can't ship 9×7 either.
const (
	minTermCols = 40
	minTermRows = 10
)

// parseResizeFrame unpacks the opcode-'1' JSON body into cols+rows.
// Accepts a frame with or without the leading opcode byte so callers
// that already stripped it still work. Floors absurdly-small values
// to a sane minimum (see minTerm*) — see the comment on those consts.
func parseResizeFrame(data []byte) (cols, rows uint32, err error) {
	body := data
	if len(body) > 0 && body[0] == termOpcodeResize {
		body = body[1:]
	}
	var r termResize
	if err := json.Unmarshal(body, &r); err != nil {
		return 0, 0, err
	}
	// Treat a missing field (0) as "use the conventional default"
	// rather than the floor — most callers that omit the field
	// genuinely want 80×24, not 20×5.
	if r.Columns == 0 {
		r.Columns = 80
	} else if r.Columns < minTermCols {
		r.Columns = minTermCols
	}
	if r.Rows == 0 {
		r.Rows = 24
	} else if r.Rows < minTermRows {
		r.Rows = minTermRows
	}
	return r.Columns, r.Rows, nil
}

// RegisterV2TerminalRoute mounts the project-scoped terminal WS
// endpoint at GET /api/v1/projects/:pid/agents/:agent_id/terminal/ws.
// Operator-tier — opening a shell on an agent is a privileged action
// even for members of the project. RequireAgentInProject blocks
// cross-project pivots through a forged agent_id.
//
// Auth uses RequireAuthWS (not RequireAuth) because browsers can't set
// Authorization headers on WebSocket upgrade requests. The browser
// passes the JWT as a "Bearer.<jwt>" Sec-WebSocket-Protocol entry; the
// middleware extracts it and stamps AccessClaims for the downstream
// RBAC checks.
func RegisterV2TerminalRoute(engine *gin.Engine, svc *core.AgentLinkService, rbac *RBAC, recMgr *recording.Manager) {
	grp := engine.Group("/api/v1/projects/:pid/agents/:agent_id")
	grp.Use(
		rbac.RequireAuthWS(),
		rbac.RequireProjectRole("pid", user.RoleOperator),
		rbac.RequireAgentInProject("pid", "agent_id"),
	)
	grp.GET("/terminal/ws", NewV2TerminalHandler(svc, recMgr))
}

// hostLookupForAgent is the package-level shim runV2Terminal uses to
// resolve the host_id behind an agent without taking a hard dep on
// *storage.DB. main.go installs a callback that closes over the live
// DB handle. Stays nil in tests that don't exercise recording.
var hostLookupForAgent func(ctx context.Context, agentID string) (string, bool)

// SetHostLookup installs the lookup callback. main.go calls this once
// after constructing storage.DB so the v2 terminal handler can stamp
// host_id on recording rows.
func SetHostLookup(fn func(ctx context.Context, agentID string) (string, bool)) {
	hostLookupForAgent = fn
}

func lookupHostIDForAgent(ctx context.Context, agentID string) (string, bool) {
	if hostLookupForAgent == nil {
		return "", false
	}
	return hostLookupForAgent(ctx, agentID)
}
