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

// host_net_* gives plugins raw outbound TCP. Framework primitive
// reserved as an extension point for future port-forward / proxy
// plugins; no system plugin currently declares CapNetDial. Three
// host fns:
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
//     until either side closes. No framing — the wire after the ack
//     is plain TCP both ways. Blocks for the connection's lifetime;
//     extism's per-plugin call serialisation guarantees no other
//     host fn races.
//
//   host_net_close(handle) -> ok|error
//     Best-effort close. Used by plugin shutdown paths so a mid-flight
//     orphaned conn gets reaped.
//
// All three require CapNetDial in the granted set.

// netDialRequest is the JSON arg for host_net_dial.
type netDialRequest struct {
	Target        string `json:"target"`
	DialTimeoutMS uint32 `json:"dial_timeout_ms,omitempty"`
}

type netDialSpawnResponse struct {
	Handle       uint32 `json:"handle"`
	ResolvedAddr string `json:"resolved_addr"`
}

// netHandle is the per-dial / per-accepted-conn state. Holds the
// live net.Conn until host_net_relay drains both directions and
// closes; closed handles are removed from the table. The same table
// also stores listener handles via netListener (kind="listener");
// host_net_close handles either kind.
type netHandle struct {
	id      uint32
	conn    net.Conn
	relayed atomic.Bool

	// listener is non-nil for entries created by host_net_listen.
	// Mutually exclusive with conn — a single handle is either an
	// accepted/dialed conn OR a listener, never both.
	listener net.Listener
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

// defaultNetDialTimeout is the dial deadline applied when the plugin
// doesn't request a specific value. Picked to be permissive enough
// for cross-region links while still bounding a stuck SYN.
const defaultNetDialTimeout = 10 * time.Second

// hostNetRelay splices the dialed conn ↔ the active stream's wire
// raw bytes until either side closes. Pure byte splice — no framing.
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
// early termination. Handles either dial-style net.Conn handles
// or listen-style net.Listener handles (the wasm doesn't have to
// remember which kind a given id was; close is symmetric).
//
// Capability check accepts CapNetDial OR CapNetListen — close is
// trust-symmetric (the plugin is just disposing of a resource it
// already created legitimately).
func (pctx *pluginCtx) hostNetClose(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapNetDial] && !pctx.granted[CapNetListen] {
		returnEnvelope(p, stack, denied("net.dial or net.listen"))
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
	if h.listener != nil {
		_ = h.listener.Close()
	}
	returnEnvelope(p, stack, envelope{Ok: true})
}

// netListenRequest is the JSON arg for host_net_listen. The bind
// addr is matched against capabilities.net.listen.binds via the
// same matchAny glob the dial allowlist uses.
type netListenRequest struct {
	Bind string `json:"bind"`
}

type netListenResponse struct {
	Handle    uint32 `json:"handle"`
	BoundAddr string `json:"bound_addr"`
}

// hostNetListen binds a TCP listener on the requested address. The
// returned handle is the input to host_net_accept; subsequent
// host_net_close releases the listener.
//
// "0.0.0.0:0" / "127.0.0.1:0" idioms work: the kernel picks a free
// port, BoundAddr in the response carries the actual port. Plugins
// that want to publish their bound address back to the operator
// (e.g. a SOCKS5 server announcing where to connect) read this.
func (pctx *pluginCtx) hostNetListen(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapNetListen] {
		returnEnvelope(p, stack, denied("net.listen"))
		return
	}
	raw, err := readStringArg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_arg: "+err.Error()))
		return
	}
	var req netListenRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		returnEnvelope(p, stack, failed("decode_request: "+err.Error()))
		return
	}
	if req.Bind == "" {
		returnEnvelope(p, stack, failed("empty_bind"))
		return
	}
	if pctx.manifest.Capabilities.NetListen == nil {
		returnEnvelope(p, stack, denied("net.listen (no manifest spec)"))
		return
	}
	if !matchAny(pctx.manifest.Capabilities.NetListen.Binds, req.Bind, 0) {
		returnEnvelope(p, stack, denied("bind_not_in_allowlist: "+req.Bind))
		return
	}

	ln, err := net.Listen("tcp", req.Bind)
	if err != nil {
		returnEnvelope(p, stack, failed("listen: "+err.Error()))
		return
	}

	pctx.netMu.Lock()
	if pctx.netHandles == nil {
		pctx.netHandles = make(map[uint32]*netHandle)
	}
	h := &netHandle{
		id:       pctx.nextNetHandleID(),
		listener: ln,
	}
	pctx.netHandles[h.id] = h
	pctx.netMu.Unlock()

	returnEnvelope(p, stack, okData(netListenResponse{
		Handle:    h.id,
		BoundAddr: ln.Addr().String(),
	}))
}

type netAcceptResponse struct {
	ConnHandle uint32 `json:"conn_handle"`
	PeerAddr   string `json:"peer_addr"`
}

// hostNetAccept blocks on the listener identified by `handle` until
// the next inbound connection arrives, then registers a fresh
// netHandle for the accepted conn. The plugin then drives the conn
// via host_net_relay (existing).
//
// The accept blocks indefinitely — extism's per-plugin call
// serialisation means no other host fn can run on the same plugin
// concurrently. A plugin wanting to "accept loop while doing other
// work" needs to architect around this (close the listener from
// another invocation context, or use multiple plugin instances).
//
// Per-conn deadline / closing: not implemented in v1. Plugin can
// host_net_close the listener to break the accept (which returns
// "use of closed network connection" up through the wasm side).
//
// Argument shape mirrors host_net_relay / host_net_close: a bare
// uint32 handle as a JSON number string (not an envelope). Keeps
// the SDK's shape uniform across the net family.
func (pctx *pluginCtx) hostNetAccept(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapNetListen] {
		returnEnvelope(p, stack, denied("net.listen"))
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
	if h == nil || h.listener == nil {
		returnEnvelope(p, stack, failed("unknown_listener_handle"))
		return
	}

	conn, err := h.listener.Accept()
	if err != nil {
		// listener.Close() returns "use of closed network
		// connection"; let the wasm see the underlying error so it
		// can decide whether to retry / unwind.
		returnEnvelope(p, stack, failed("accept: "+err.Error()))
		return
	}

	pctx.netMu.Lock()
	connHandle := &netHandle{
		id:   pctx.nextNetHandleID(),
		conn: conn,
	}
	pctx.netHandles[connHandle.id] = connHandle
	pctx.netMu.Unlock()

	returnEnvelope(p, stack, okData(netAcceptResponse{
		ConnHandle: connHandle.id,
		PeerAddr:   conn.RemoteAddr().String(),
	}))
}

// reapNetHandles iterates any orphaned conns + listeners and closes
// them. Called by loaded.close so a plugin that crashed mid-relay
// doesn't leak a long-lived TCP conn or a bound port.
func (pctx *pluginCtx) reapNetHandles() {
	pctx.netMu.Lock()
	handles := pctx.netHandles
	pctx.netHandles = nil
	pctx.netMu.Unlock()
	for _, h := range handles {
		if h.conn != nil {
			_ = h.conn.Close()
		}
		if h.listener != nil {
			_ = h.listener.Close()
		}
	}
}

// (silence unused-decl for fmt; keeping it on hand for richer error
// returns once the relay's failure surface gets exercised.)
var _ = fmt.Errorf
