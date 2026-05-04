package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	extism "github.com/extism/go-sdk"
)

// host_net_* gives plugins outbound TCP — the wasm migration target
// for the legacy STREAM_TYPE_TUNNEL_PULL handler. Three host fns:
//
//   host_net_dial(spec_json) -> {handle, resolved_addr}
//     Validates the requested target against the manifest's
//     capabilities.net.dial.targets allowlist, dials over TCP with a
//     bounded timeout, records a per-plugin handle, returns the
//     resolved peer address (useful for the audit log + the operator
//     UI's "actual address dialed" surface).
//
//   host_net_relay(handle) -> ok|error
//     Splices the dialed conn ↔ the active stream's wire raw bytes
//     until either side closes. No framing — tunnel_pull's wire
//     after the ack is plain TCP both ways. Blocks for the connection's
//     lifetime; extism's per-plugin call serialisation guarantees no
//     other host fn races.
//
//   host_net_close(handle) -> ok|error
//     Best-effort close. Used by plugin shutdown paths so a mid-flight
//     orphaned conn gets reaped.
//
// All three require CapNetDial in the granted set.

// netDialRequest is the JSON arg for host_net_dial. Mirrors
// v2pb.TunnelPullRequest's two fields.
type netDialRequest struct {
	Target        string `json:"target"`
	DialTimeoutMS uint32 `json:"dial_timeout_ms,omitempty"`
}

type netDialSpawnResponse struct {
	Handle       uint32 `json:"handle"`
	ResolvedAddr string `json:"resolved_addr"`
}

// netHandle is the per-dial state. Holds the live net.Conn until
// host_net_relay drains both directions and closes; closed handles
// are removed from the table.
type netHandle struct {
	id      uint32
	conn    net.Conn
	relayed atomic.Bool
}

func (pctx *pluginCtx) nextNetHandleID() uint32 {
	return atomic.AddUint32(&pctx.netHandleCounter, 1)
}

// hostNetDial validates + dials. Returns {handle, resolved_addr} on
// success.
func (pctx *pluginCtx) hostNetDial(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapNetDial] {
		returnEnvelope(p, stack, denied("net.dial"))
		return
	}
	raw, err := readStringArg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_arg: "+err.Error()))
		return
	}
	var req netDialRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		returnEnvelope(p, stack, failed("decode_request: "+err.Error()))
		return
	}
	if req.Target == "" {
		returnEnvelope(p, stack, failed("empty_target"))
		return
	}
	if pctx.manifest.Capabilities.NetDial == nil {
		returnEnvelope(p, stack, denied("net.dial (no manifest spec)"))
		return
	}
	// matchAny accepts glob patterns (`*.example.com:22`,
	// `10.0.0.?:443`) on top of the literal-or-`*` form. pathSep=0
	// because targets are host:port pairs, not paths — `*` means any
	// chars including `.` and `:`.
	if !matchAny(pctx.manifest.Capabilities.NetDial.Targets, req.Target, 0) {
		returnEnvelope(p, stack, denied("target_not_in_allowlist: "+req.Target))
		return
	}

	timeout := defaultNetDialTimeout
	if req.DialTimeoutMS > 0 {
		timeout = time.Duration(req.DialTimeoutMS) * time.Millisecond
	}
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(dialCtx, "tcp", req.Target)
	if err != nil {
		returnEnvelope(p, stack, failed("dial: "+err.Error()))
		return
	}

	pctx.netMu.Lock()
	if pctx.netHandles == nil {
		pctx.netHandles = make(map[uint32]*netHandle)
	}
	h := &netHandle{
		id:   pctx.nextNetHandleID(),
		conn: conn,
	}
	pctx.netHandles[h.id] = h
	pctx.netMu.Unlock()

	returnEnvelope(p, stack, okData(netDialSpawnResponse{
		Handle:       h.id,
		ResolvedAddr: conn.RemoteAddr().String(),
	}))
}

// defaultNetDialTimeout matches the legacy
// internal/agent/tunnel_pull_stream.go's
// defaultTunnelDialTimeout — kept consistent so a
// migration-day cutover doesn't change the apparent dial behaviour.
const defaultNetDialTimeout = 10 * time.Second

// hostNetRelay splices the dialed conn ↔ the active stream's wire
// raw bytes until either side closes. Pure byte splice — no framing.
// Mirrors the io.Copy pair in
// internal/agent/tunnel_pull_stream.go's spliceBidirectional.
func (pctx *pluginCtx) hostNetRelay(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapNetDial] {
		returnEnvelope(p, stack, denied("net.dial"))
		return
	}
	id, err := readUint32Arg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_arg: "+err.Error()))
		return
	}
	pctx.netMu.Lock()
	h := pctx.netHandles[id]
	pctx.netMu.Unlock()
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

	// Bidirectional splice. Two goroutines, whichever side finishes
	// first triggers the other to unwind. Wire side closes via
	// h.conn.Close() (closing the dialed conn unblocks the
	// wire→conn copy on its next read). The wire itself has no
	// Close method available here (it's the bridge wire), so the
	// dispatcher's deferred stream.Close handles its teardown.
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(h.conn, wire)
		_ = h.conn.Close()
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(wire, h.conn)
		done <- struct{}{}
	}()
	<-done
	<-done

	pctx.netMu.Lock()
	delete(pctx.netHandles, id)
	pctx.netMu.Unlock()

	returnEnvelope(p, stack, envelope{Ok: true})
}

// hostNetClose best-effort closes a handle. Mostly defensive — relay
// is the normal happy-path exit; close is for the wasm wanting an
// early termination.
func (pctx *pluginCtx) hostNetClose(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapNetDial] {
		returnEnvelope(p, stack, denied("net.dial"))
		return
	}
	id, err := readUint32Arg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_arg: "+err.Error()))
		return
	}
	pctx.netMu.Lock()
	h := pctx.netHandles[id]
	delete(pctx.netHandles, id)
	pctx.netMu.Unlock()
	if h == nil {
		returnEnvelope(p, stack, failed("unknown_handle"))
		return
	}
	if h.conn != nil {
		_ = h.conn.Close()
	}
	returnEnvelope(p, stack, envelope{Ok: true})
}

// reapNetHandles iterates any orphaned conns and closes them. Called
// by loaded.close so a plugin that crashed mid-relay doesn't leak a
// long-lived TCP conn.
func (pctx *pluginCtx) reapNetHandles() {
	pctx.netMu.Lock()
	handles := pctx.netHandles
	pctx.netHandles = nil
	pctx.netMu.Unlock()
	for _, h := range handles {
		if h.conn != nil {
			_ = h.conn.Close()
		}
	}
}

// (silence unused-decl for fmt; keeping it on hand for richer error
// returns once the relay's failure surface gets exercised.)
var _ = fmt.Errorf
