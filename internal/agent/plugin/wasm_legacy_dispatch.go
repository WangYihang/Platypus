package plugin

import (
	"context"
	"fmt"
	"io"

	"github.com/WangYihang/Platypus/internal/log"
)

// DispatchLegacyWasmStream is the dispatch path for plugins that
// claim a legacy stream type (FILE_READ, FILE_WRITE, PROCESS_OPEN,
// …) with a `wasm:method` host_handler instead of the host-provider
// `agent.X` marker. Bridges the wire protocol the legacy Go handlers
// emit (length-prefixed proto frames via internal/link.WriteFrame)
// to a wasm method that owns the byte production.
//
// Differences from DispatchPluginStream (the PLUGIN_STREAM path):
//   - No PluginStreamFrame wrapping. The wire IS a sequence of
//     length-prefixed proto frames matching the existing wire
//     contract — server-side readers don't change.
//   - No inbound pump. The request is in `metadata` (the typed proto
//     bytes the server put in the stream header) and is passed
//     directly to the wasm method as input. host_stream_read on the
//     wasm side returns immediate EOF.
//   - No outbound pump. Wasm calls host_link_write_frame to write
//     length-prefixed frames straight to the wire; the streamCtx's
//     `wire` field is the destination.
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
	// channels). host_link_write_frame consults pctx.activeStream()
	// and writes to s.wire directly.
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
