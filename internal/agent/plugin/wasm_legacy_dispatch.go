package plugin

import (
	"context"
	"fmt"
	"io"

	"github.com/WangYihang/Platypus/internal/log"
)

// DispatchLegacyWasmStream is the dispatch path for typed stream
// types (FILE_READ, FILE_WRITE, PROCESS_OPEN, …) — server opens the
// stream with a typed metadata payload (FileReadRequest etc) and
// expects to read length-prefixed typed-response frames back. The
// "Legacy" in the name is historical: post-E6 the plugin reads &
// writes via the unified host_stream_*, the same host fns the
// PLUGIN_STREAM path uses; the dispatcher just runs in raw-wire
// mode (streamCtx.wire != nil) so host_stream_* skips its envelope
// wrapping and operates directly on the wire.
//
// Differences from DispatchPluginStream (the PLUGIN_STREAM path):
//   - No PluginStreamFrame wrapping. The wire IS a sequence of
//     length-prefixed proto frames matching the existing wire
//     contract — server-side readers don't change.
//   - No inbound pump. The request is in `metadata` (the typed
//     proto bytes the server put in the stream header) and is
//     passed directly to the wasm method as input. host_stream_read
//     in raw-wire mode reads any subsequent inbound frames the
//     server emits (the FILE_WRITE chunks, in particular).
//   - No outbound pump. Wasm calls host_stream_write which, in
//     raw-wire mode, emits length-prefixed frames straight to the
//     wire; the streamCtx's `wire` field is the destination.
//
// Acquires loaded.mu for the wasm call's duration — extism plugins
// are not goroutine-safe, so concurrent streams (or a stream + an
// Invoke) on the same plugin would corrupt state.
func (r *Registry) DispatchLegacyWasmStream(
	ctx context.Context,
	stream io.ReadWriteCloser,
	pluginID, method string,
	metadata []byte,
) error {
	defer func() { _ = stream.Close() }()

	r.mu.RLock()
	l, ok := r.plugins[pluginID]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("plugin: legacy-wasm dispatch: plugin %q not installed", pluginID)
	}
	if !l.entry.Enabled {
		return fmt.Errorf("plugin: legacy-wasm dispatch: plugin %q disabled", pluginID)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	inst, err := l.instanceOf(ctx)
	if err != nil {
		return fmt.Errorf("plugin: legacy-wasm dispatch: instantiate %q: %w", pluginID, err)
	}

	// Set the active stream slot to a wire-backed streamCtx (no
	// channels). host_stream_read / host_stream_write consult
	// pctx.activeStream() and operate on s.wire directly.
	s := &streamCtx{wire: stream}
	l.pctx.setActiveStream(s)
	defer l.pctx.clearActiveStream()

	_, _, callErr := inst.CallWithContext(ctx, method, metadata)
	if callErr != nil {
		log.L.Warn("plugin.legacy_wasm.invoke_failed",
			"plugin_id", pluginID,
			"method", method,
			"error", callErr.Error(),
		)
		return callErr
	}
	return nil
}
