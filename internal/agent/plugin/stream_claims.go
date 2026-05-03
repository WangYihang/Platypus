package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// StreamClaim is the per-stream-type ownership record the plugin
// runtime exposes to the agent's stream dispatcher. A registered
// claim displaces the legacy hardcoded handler for that stream type;
// the agent's serve_link.go consults Registry.StreamClaim before
// falling into its own switch.
//
// MVP shape: every claim resolves to a host-side StreamProvider
// (registered via SetStreamProvider). Wasm-mediated stream IO — where
// the bytes themselves flow through the plugin's wasm — is the
// bigger Phase 2 work described in docs/plugins/STREAMING_ABI.md and
// will reuse the same StreamClaim type with a HostHandler that maps
// to a wasm dispatch instead of a host fn.
type StreamClaim struct {
	PluginID    string // owner
	HostHandler string // provider name to delegate to
	StreamName  string // plugin-author-facing label, for audit
}

// StreamProvider is the host-side function that runs the stream's
// actual IO. The agent main wires the legacy stream handlers
// (HandleProcessStream, HandleFileReadStream, ...) as named
// providers; the plugin's manifest references the names via
// host_handler.
//
// metadata is the StreamHeader.metadata bytes the agent received off
// the wire; the provider unmarshals it into the per-stream-type
// proto request itself (FileReadRequest, ProcessOpenRequest, ...).
// Centralising the unmarshal in the provider keeps this dispatch
// layer schema-agnostic — it doesn't need to know what type each
// stream carries.
type StreamProvider func(ctx context.Context, stream io.ReadWriteCloser, metadata []byte) error

// streamProviders is the process-wide name → provider map. Lookups
// happen on every stream dispatch so it's an RWMutex to keep the hot
// path lock-free for reads.
var (
	streamProvidersMu sync.RWMutex
	streamProviders   = map[string]StreamProvider{}
)

// SetStreamProvider registers (or replaces) a host-side stream
// handler. Names are case-sensitive plugin-author-visible strings;
// the agent main wires conventional names like "agent.process",
// "agent.file_read", etc.
func SetStreamProvider(name string, p StreamProvider) {
	streamProvidersMu.Lock()
	streamProviders[name] = p
	streamProvidersMu.Unlock()
}

// LookupStreamProvider returns the registered provider, or
// (nil, false) when the name is unknown.
func LookupStreamProvider(name string) (StreamProvider, bool) {
	streamProvidersMu.RLock()
	p, ok := streamProviders[name]
	streamProvidersMu.RUnlock()
	return p, ok
}

// ResetStreamProvidersForTest empties the registry. Tests use it to
// isolate provider registrations between cases. Not exported in the
// non-test sense (keep the whole-process registry intentional).
func ResetStreamProvidersForTest() {
	streamProvidersMu.Lock()
	streamProviders = map[string]StreamProvider{}
	streamProvidersMu.Unlock()
}

// StreamClaim is the per-StreamType lookup the agent dispatcher
// uses. Returns the claim + whether a registered provider is
// available; when the provider is missing the dispatcher SHOULD
// fall through to its legacy handler so the stream isn't lost.
func (r *Registry) StreamClaim(t v2pb.StreamType) (claim StreamClaim, providerOK bool, ok bool) {
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
			c := StreamClaim{
				PluginID:    l.id,
				HostHandler: s.HostHandler,
				StreamName:  s.Name,
			}
			_, providerOK = LookupStreamProvider(s.HostHandler)
			return c, providerOK, true
		}
	}
	return StreamClaim{}, false, false
}

// DispatchStream is the one-call entry the agent dispatcher uses.
// Looks up the claim, resolves the provider, and runs it. Returns
// (true, err) when the dispatch was handled by a plugin (err is the
// provider's return); (false, nil) when no plugin claimed the
// stream (the dispatcher should fall through to its legacy switch);
// (true, error) for the in-between cases (claim found but provider
// missing) — surfaced as an error so a misconfigured deployment
// fails loudly instead of silently routing to the wrong place.
//
// STREAM_TYPE_PLUGIN_STREAM is the wasm-mediated path: it bypasses
// the per-type claim lookup (no manifest declares the generic slot)
// and routes straight to DispatchPluginStream, which parses the
// embedded PluginStreamRequest header to find the target plugin +
// stream name.
func (r *Registry) DispatchStream(ctx context.Context, t v2pb.StreamType, stream io.ReadWriteCloser, metadata []byte) (handled bool, err error) {
	if t == v2pb.StreamType_STREAM_TYPE_PLUGIN_STREAM {
		return true, r.DispatchPluginStream(ctx, stream, metadata)
	}
	claim, providerOK, ok := r.StreamClaim(t)
	if !ok {
		return false, nil
	}
	// `wasm:method` host_handler markers route through the legacy-
	// wasm bridge — the wasm method owns frame production via
	// host_link_write_frame, the wire protocol stays byte-for-byte
	// identical to what the legacy Go handlers used to emit. No
	// StreamProvider lookup happens for these (the host-provider
	// claim path is exclusive to `agent.X` markers).
	if method, isWasm := parseWasmHandler(claim.HostHandler); isWasm {
		return true, r.DispatchLegacyWasmStream(ctx, stream, claim.PluginID, method, metadata)
	}
	if !providerOK {
		return true, fmt.Errorf("plugin: stream %s: claim by %s references unknown provider %q",
			t.String(), claim.PluginID, claim.HostHandler)
	}
	provider, _ := LookupStreamProvider(claim.HostHandler)
	return true, provider(ctx, stream, metadata)
}

// errNoClaim is the sentinel for "no plugin claimed this type". Kept
// as a named error so callers can distinguish "fall through to
// legacy" from real failures.
var errNoClaim = errors.New("plugin: no claim for stream type")
