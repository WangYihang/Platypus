package plugin

import (
	"context"
	"fmt"
	"io"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// StreamClaim is the per-stream-type ownership record the plugin
// runtime exposes to the agent's stream dispatcher. A registered
// claim displaces the agent's built-in switch for that stream type;
// serve_link.go consults Registry.DispatchStream before falling
// through to its own (RPC / upgrade / plugin-mgmt) handlers.
//
// HostHandler is a wasm-dispatch marker of the form "wasm:<method>",
// pointing at the wasm export the plugin exposes for this stream.
// The legacy `agent.X` host-provider markers are gone — every
// remaining claim runs in-sandbox.
type StreamClaim struct {
	PluginID    string // owner
	HostHandler string // "wasm:<method_name>" — see wasm_handler.go
	StreamName  string // plugin-author-facing label, for audit
}

// ClaimedStreamTypes returns the set of stream-type strings that
// at least one enabled plugin in the registry claims. Used at
// agent boot to log which capabilities the operator's baseline
// allowlist actually covers — and warn about which legacy types
// don't have a wasm replacement installed.
//
// Returned set is keyed by the v2pb.StreamType.String() form
// ("STREAM_TYPE_FILE_READ", …) so callers can range over the
// enum's known names without a separate lookup.
func (r *Registry) ClaimedStreamTypes() map[string]struct{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]struct{})
	for _, l := range r.plugins {
		if !l.entry.Enabled {
			continue
		}
		for _, s := range l.manifest.Streams {
			if s.StreamType != "" {
				out[s.StreamType] = struct{}{}
			}
		}
	}
	return out
}

// StreamClaim is the per-StreamType lookup the agent dispatcher
// uses. Returns the claim and whether one was found. ok=false means
// no enabled plugin owns this type (the dispatcher falls through to
// its built-in switch in serve_link.go).
func (r *Registry) StreamClaim(t v2pb.StreamType) (claim StreamClaim, ok bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, l := range r.plugins {
		if !l.entry.Enabled {
			continue
		}
		for _, s := range l.manifest.Streams {
			if s.StreamType != t.String() {
				continue
			}
			return StreamClaim{
				PluginID:    l.id,
				HostHandler: s.HostHandler,
				StreamName:  s.Name,
			}, true
		}
	}
	return StreamClaim{}, false
}

// DispatchStream is the one-call entry the agent dispatcher uses.
// Looks up the claim and routes the stream into the wasm sandbox.
// Returns:
//
//   - (true, nil) — plugin owned + ran the stream successfully
//   - (true, err) — plugin owned the stream but errored mid-dispatch
//     (malformed metadata, wasm trap, missing export …)
//   - (false, nil) — no plugin claimed this type; the agent
//     dispatcher should fall through to its built-in switch
//
// STREAM_TYPE_PLUGIN_STREAM is the wasm-mediated bidirectional path:
// it bypasses the per-type claim lookup (no manifest declares the
// generic slot) and routes straight to DispatchPluginStream, which
// parses the embedded PluginStreamRequest header to find the target
// plugin + stream name.
func (r *Registry) DispatchStream(ctx context.Context, t v2pb.StreamType, stream io.ReadWriteCloser, metadata []byte) (handled bool, err error) {
	if t == v2pb.StreamType_STREAM_TYPE_PLUGIN_STREAM {
		return true, r.DispatchPluginStream(ctx, stream, metadata)
	}
	claim, ok := r.StreamClaim(t)
	if !ok {
		return false, nil
	}
	method, isWasm := parseWasmHandler(claim.HostHandler)
	if !isWasm {
		// Manifest validation should have caught this at install time;
		// surfacing it loudly here defends against an out-of-band edit
		// or a future host_handler form we haven't taught the
		// dispatcher about yet.
		return true, fmt.Errorf("plugin: stream %s: claim by %s has unsupported host_handler %q",
			t.String(), claim.PluginID, claim.HostHandler)
	}
	return true, r.DispatchLegacyWasmStream(ctx, stream, claim.PluginID, method, metadata)
}
