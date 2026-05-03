package plugin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/creack/pty"
	extism "github.com/extism/go-sdk"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// host_process_* gives plugins streaming process spawn — the wasm
// migration target for the legacy STREAM_TYPE_PROCESS_OPEN handler.
// Three host fns:
//
//   host_process_spawn(spec_json) -> {handle_id, pid}
//     Validates the requested command against the manifest's
//     capabilities.process.commands allowlist, spawns a child
//     (PTY-backed when spec.pty=true), records a per-plugin handle,
//     returns the OS pid.
//
//   host_process_relay(handle_id) -> {exit_code, signal, error}
//     Runs the bidirectional pump on the host: server-side wire ↔
//     child stdin/stdout/stderr. Blocks until the child exits or
//     the wire closes. The wasm method is single-threaded so no
//     other host fn runs on this plugin during the relay; the host
//     owns the wire bytes for the relay's lifetime.
//
//   host_process_kill(handle_id) -> ok|error
//     Best-effort kill. Mostly used by plugin shutdown paths so a
//     mid-flight orphaned process gets reaped.
//
// All three require CapProcess in the granted set.
//
// Per-plugin handle table lives on pluginCtx; cleanup runs when the
// plugin instance is torn down. Handles are uint32 not uint64 so the
// JSON envelope stays small.

// processHandleRequest is the JSON arg for host_process_spawn.
// Mirrors v2pb.ProcessOpenRequest's fields, hand-modeled here so the
// wasm side doesn't need to drag in proto codegen.
type processHandleRequest struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Cwd     string            `json:"cwd,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Pty     bool              `json:"pty,omitempty"`
	Cols    uint32            `json:"cols,omitempty"`
	Rows    uint32            `json:"rows,omitempty"`
}

type processHandleSpawnResponse struct {
	Handle uint32 `json:"handle"`
	Pid    int32  `json:"pid"`
}

type processHandleRelayResponse struct {
	ExitCode int32  `json:"exit_code"`
	Signal   string `json:"signal,omitempty"`
	Error    string `json:"error,omitempty"`
}

// processHandle is the per-spawn agent-side state. Either ptmx is set
// (PTY mode) or the three pipe fields are (plain mode).
type processHandle struct {
	id         uint32
	cmd        *exec.Cmd
	ptmx       *os.File
	stdin      io.WriteCloser
	stdoutPipe io.ReadCloser
	stderrPipe io.ReadCloser
	relayed    atomic.Bool // set true on the first relay; relay is one-shot
	killed     atomic.Bool
}

// nextProcessHandleID is plugin-scoped (lives on pluginCtx). Wrapped
// to a method so concurrent spawns from a single plugin (which can't
// happen today but might someday) stay safe.
func (pctx *pluginCtx) nextProcessHandleID() uint32 {
	return atomic.AddUint32(&pctx.processHandleCounter, 1)
}

// hostProcessSpawn validates + spawns. Returns {handle, pid} on
// success; envelope.error otherwise. The spawn is rejected without
// touching the OS if the manifest's allowlist doesn't include the
// requested command (mirrors host_exec's check).
func (pctx *pluginCtx) hostProcessSpawn(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapProcess] {
		returnEnvelope(p, stack, denied("process"))
		return
	}
	raw, err := readStringArg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_arg: "+err.Error()))
		return
	}
	var req processHandleRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		returnEnvelope(p, stack, failed("decode_request: "+err.Error()))
		return
	}
	if req.Command == "" {
		returnEnvelope(p, stack, failed("empty_command"))
		return
	}
	if pctx.manifest.Capabilities.Process == nil {
		returnEnvelope(p, stack, denied("process (no manifest spec)"))
		return
	}
	allowed := false
	for _, c := range pctx.manifest.Capabilities.Process.Commands {
		if c == "*" || c == req.Command {
			allowed = true
			break
		}
	}
	if !allowed {
		returnEnvelope(p, stack, denied("command_not_in_allowlist: "+req.Command))
		return
	}

	cmd := exec.Command(req.Command, req.Args...)
	if req.Cwd != "" {
		cmd.Dir = req.Cwd
	}
	if len(req.Env) > 0 {
		cmd.Env = append(os.Environ(), envSliceFromMap(req.Env)...)
	}

	h := &processHandle{cmd: cmd}
	if req.Pty {
		ws := &pty.Winsize{Cols: uint16(req.Cols), Rows: uint16(req.Rows)}
		ptmx, perr := pty.StartWithSize(cmd, ws)
		if perr != nil {
			returnEnvelope(p, stack, failed("spawn_pty: "+perr.Error()))
			return
		}
		h.ptmx = ptmx
	} else {
		stdin, perr := cmd.StdinPipe()
		if perr != nil {
			returnEnvelope(p, stack, failed("stdin_pipe: "+perr.Error()))
			return
		}
		stdoutP, perr := cmd.StdoutPipe()
		if perr != nil {
			returnEnvelope(p, stack, failed("stdout_pipe: "+perr.Error()))
			return
		}
		stderrP, perr := cmd.StderrPipe()
		if perr != nil {
			returnEnvelope(p, stack, failed("stderr_pipe: "+perr.Error()))
			return
		}
		if perr := cmd.Start(); perr != nil {
			returnEnvelope(p, stack, failed("spawn: "+perr.Error()))
			return
		}
		h.stdin = stdin
		h.stdoutPipe = stdoutP
		h.stderrPipe = stderrP
	}

	pctx.processMu.Lock()
	if pctx.processHandles == nil {
		pctx.processHandles = make(map[uint32]*processHandle)
	}
	h.id = pctx.nextProcessHandleID()
	pctx.processHandles[h.id] = h
	pctx.processMu.Unlock()

	pid := int32(0)
	if cmd.Process != nil {
		pid = int32(cmd.Process.Pid)
	}
	returnEnvelope(p, stack, okData(processHandleSpawnResponse{Handle: h.id, Pid: pid}))
}

// hostProcessRelay drives the bidirectional pump. The wire is
// extracted from the active stream slot — the bridge dispatcher
// (DispatchLegacyWasmStream) sets it to the underlying agent stream.
// Returns when the child exits OR the wire closes.
func (pctx *pluginCtx) hostProcessRelay(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapProcess] {
		returnEnvelope(p, stack, denied("process"))
		return
	}
	id, err := readUint32Arg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_arg: "+err.Error()))
		return
	}
	pctx.processMu.Lock()
	h := pctx.processHandles[id]
	pctx.processMu.Unlock()
	if h == nil {
		returnEnvelope(p, stack, failed("unknown_handle"))
		return
	}
	if !h.relayed.CompareAndSwap(false, true) {
		returnEnvelope(p, stack, failed("already_relayed"))
		return
	}
	s := pctx.activeStream()
	if s == nil || s.wire == nil {
		returnEnvelope(p, stack, failed("stream_not_legacy_bridge"))
		return
	}
	wire := s.wire

	resp := relayProcess(ctx, wire, h)

	// Cleanup the handle entry — it's exhausted now (the child has
	// exited and the relay drained both directions). Leaving the
	// entry would just clutter the table; the wasm can re-spawn.
	pctx.processMu.Lock()
	delete(pctx.processHandles, id)
	pctx.processMu.Unlock()

	returnEnvelope(p, stack, okData(resp))
}

// hostProcessKill best-effort kills a handle. Mostly defensive — the
// relay is the normal happy-path exit; kill is for the wasm panicking
// or wanting an early termination.
func (pctx *pluginCtx) hostProcessKill(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapProcess] {
		returnEnvelope(p, stack, denied("process"))
		return
	}
	id, err := readUint32Arg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_arg: "+err.Error()))
		return
	}
	pctx.processMu.Lock()
	h := pctx.processHandles[id]
	pctx.processMu.Unlock()
	if h == nil {
		returnEnvelope(p, stack, failed("unknown_handle"))
		return
	}
	h.killed.Store(true)
	if h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
	}
	returnEnvelope(p, stack, envelope{Ok: true})
}

// relayProcess runs the bidirectional pump on the wire ↔ child. Same
// semantics as the legacy process_stream.go's pumpPTY / pumpPipes,
// hoisted here so the wasm bridge owns the byte-pumping
// infrastructure while the wasm plugin owns spawn policy. Blocks
// until the child exits.
func relayProcess(ctx context.Context, wire io.ReadWriter, h *processHandle) processHandleRelayResponse {
	if h.ptmx != nil {
		return relayPTY(ctx, wire, h)
	}
	return relayPipes(ctx, wire, h)
}

func relayPTY(ctx context.Context, wire io.ReadWriter, h *processHandle) processHandleRelayResponse {
	var writeMu sync.Mutex
	write := func(f *v2pb.ProcessFrame) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return link.WriteFrame(wire, f)
	}

	// server -> PTY
	go func() {
		for {
			var f v2pb.ProcessFrame
			if err := link.ReadFrame(wire, &f); err != nil {
				if h.cmd != nil && h.cmd.Process != nil && !h.killed.Load() {
					_ = h.cmd.Process.Kill()
				}
				return
			}
			switch p := f.Payload.(type) {
			case *v2pb.ProcessFrame_Stdin:
				if _, err := h.ptmx.Write(p.Stdin); err != nil {
					return
				}
			case *v2pb.ProcessFrame_Resize:
				_ = pty.Setsize(h.ptmx, &pty.Winsize{
					Cols: uint16(p.Resize.Cols),
					Rows: uint16(p.Resize.Rows),
				})
			}
		}
	}()

	// PTY -> server
	ptyDone := make(chan struct{})
	go func() {
		defer close(ptyDone)
		buf := make([]byte, 4096)
		for {
			n, err := h.ptmx.Read(buf)
			if n > 0 {
				if wErr := write(&v2pb.ProcessFrame{
					Payload: &v2pb.ProcessFrame_Stdout{Stdout: append([]byte(nil), buf[:n]...)},
				}); wErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	waitErr := h.cmd.Wait()
	_ = h.ptmx.Close()
	select {
	case <-ptyDone:
	case <-ctx.Done():
	}

	exit := waitInfoFor(h.cmd, waitErr)
	_ = write(&v2pb.ProcessFrame{Payload: &v2pb.ProcessFrame_Exit{Exit: exit}})
	return processHandleRelayResponse{
		ExitCode: exit.GetCode(),
		Signal:   exit.GetSignal(),
	}
}

func relayPipes(ctx context.Context, wire io.ReadWriter, h *processHandle) processHandleRelayResponse {
	var writeMu sync.Mutex
	write := func(f *v2pb.ProcessFrame) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return link.WriteFrame(wire, f)
	}

	// server -> stdin
	go func() {
		for {
			var f v2pb.ProcessFrame
			if err := link.ReadFrame(wire, &f); err != nil {
				_ = h.stdin.Close()
				return
			}
			if p, ok := f.Payload.(*v2pb.ProcessFrame_Stdin); ok {
				if _, err := h.stdin.Write(p.Stdin); err != nil {
					return
				}
			}
		}
	}()

	stdoutDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		buf := make([]byte, 4096)
		for {
			n, err := h.stdoutPipe.Read(buf)
			if n > 0 {
				if wErr := write(&v2pb.ProcessFrame{
					Payload: &v2pb.ProcessFrame_Stdout{Stdout: append([]byte(nil), buf[:n]...)},
				}); wErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		buf := make([]byte, 4096)
		for {
			n, err := h.stderrPipe.Read(buf)
			if n > 0 {
				if wErr := write(&v2pb.ProcessFrame{
					Payload: &v2pb.ProcessFrame_Stderr{Stderr: append([]byte(nil), buf[:n]...)},
				}); wErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	waitErr := h.cmd.Wait()
	select {
	case <-stdoutDone:
	case <-ctx.Done():
	}
	select {
	case <-stderrDone:
	case <-ctx.Done():
	}

	exit := waitInfoFor(h.cmd, waitErr)
	_ = write(&v2pb.ProcessFrame{Payload: &v2pb.ProcessFrame_Exit{Exit: exit}})
	return processHandleRelayResponse{
		ExitCode: exit.GetCode(),
		Signal:   exit.GetSignal(),
	}
}

// waitInfoFor turns os/exec's wait result into the proto ExitInfo
// shape. Mirrors the helper in internal/agent/process_stream.go;
// duplicated here rather than imported to keep the agent → plugin
// dependency direction one-way.
//
// The proto's ExitInfo carries `code` (int32) + `signal` (string).
// We populate code from cmd.ProcessState.ExitCode(); a child killed
// by a signal lands as exit code -1 with no separate signal field
// today (matches the legacy handler's behaviour, which also dropped
// signal name on the floor).
func waitInfoFor(cmd *exec.Cmd, waitErr error) *v2pb.ExitInfo {
	info := &v2pb.ExitInfo{}
	if cmd.ProcessState != nil {
		info.Code = int32(cmd.ProcessState.ExitCode())
	} else if waitErr != nil {
		info.Code = -1
	}
	return info
}

func envSliceFromMap(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

// readUint32Arg reads a JSON-encoded uint32 from the wasm stack
// position. Used for the handle_id arguments to the relay/kill fns.
func readUint32Arg(p *extism.CurrentPlugin, ptr uint64) (uint32, error) {
	raw, err := readStringArg(p, ptr)
	if err != nil {
		return 0, err
	}
	var v uint32
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return 0, fmt.Errorf("parse uint32: %w", err)
	}
	return v, nil
}

// reaperOnClose iterates any orphaned handles and kills them. Called
// by Registry.Close (or per-plugin teardown) so a plugin that crashed
// mid-relay doesn't leak a child.
func (pctx *pluginCtx) reapProcessHandles() {
	pctx.processMu.Lock()
	handles := pctx.processHandles
	pctx.processHandles = nil
	pctx.processMu.Unlock()
	for _, h := range handles {
		if h.cmd != nil && h.cmd.Process != nil {
			_ = h.cmd.Process.Kill()
		}
	}
}

// (silence unused-decl if base64 path lands in a follow-up — the
// import is still needed because future write_stdin paths will want
// raw bytes.)
var _ = base64.StdEncoding
