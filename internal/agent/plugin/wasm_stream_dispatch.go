package plugin

import (
	"context"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// DispatchPluginStream is the agent dispatcher's entry point for
// STREAM_TYPE_PLUGIN_STREAM. The wire metadata is a marshalled
// PluginStreamRequest identifying which plugin + which named
// streams[] entry to invoke; the inner request bytes (e.g.
// FileReadRequest) ride along in PluginStreamRequest.payload.
//
// The function:
//   1. Parses the metadata + looks up the plugin in the registry.
//   2. Verifies the plugin is enabled + declares the requested
//      stream name + the manifest's host_handler is the wasm:
//      marker. The legacy claim path (host_handler="agent.X") is
//      explicitly NOT accepted — it has its own dispatcher.
//   3. Allocates per-stream channels via runActiveStream + spawns
//      pumpInboundFrames + pumpOutboundFrames around the wasm
//      method invocation.
//   4. After the wasm method returns, joins the pumps + writes the
//      terminal frame so the wire peer sees a clean close.
//
// Error returns are surfaced to the caller AND emitted as KIND_ERROR
// frames on the wire so the operator's UI sees a structured failure
// rather than an unexplained connection drop.
func (r *Registry) DispatchPluginStream(ctx context.Context, stream io.ReadWriteCloser, metadata []byte) error {
	defer func() { _ = stream.Close() }()

	var req v2pb.PluginStreamRequest
	if err := proto.Unmarshal(metadata, &req); err != nil {
		return wireError(stream, "parse_metadata", err.Error())
	}

	r.mu.RLock()
	l, ok := r.plugins[req.GetPluginId()]
	r.mu.RUnlock()
	if !ok {
		return wireError(stream, "plugin_not_installed", req.GetPluginId())
	}
	if !l.entry.Enabled {
		return wireError(stream, "plugin_disabled", req.GetPluginId())
	}

	var manifestEntry *ManifestStream
	for i := range l.manifest.Streams {
		if l.manifest.Streams[i].Name == req.GetStreamName() {
			manifestEntry = &l.manifest.Streams[i]
			break
		}
	}
	if manifestEntry == nil {
		return wireError(stream, "stream_not_declared",
			fmt.Sprintf("plugin %s has no streams[].name=%q",
				req.GetPluginId(), req.GetStreamName()))
	}
	method, isWasm := parseWasmHandler(manifestEntry.HostHandler)
	if !isWasm {
		return wireError(stream, "non_wasm_handler",
			fmt.Sprintf("plugin %s stream %q host_handler=%q is not a wasm: marker; this dispatcher is wasm-only",
				req.GetPluginId(), req.GetStreamName(), manifestEntry.HostHandler))
	}

	// Acquire l.mu for the duration of the wasm call — extism's
	// Plugin is not goroutine-safe so two concurrent streams (or
	// a stream + an Invoke) on the same plugin would corrupt state.
	l.mu.Lock()
	defer l.mu.Unlock()

	inst, err := l.instanceOf(ctx)
	if err != nil {
		return wireError(stream, "instantiate", err.Error())
	}

	// Build the wasm invoker that wraps extism.Plugin.CallWithContext.
	// runActiveStream sets pctx.activeStream before invoker runs so
	// the host_stream_* primitives find a valid streamCtx.
	invoker := func(ctx context.Context, methodName string, input []byte) ([]byte, error) {
		_, out, err := inst.CallWithContext(ctx, methodName, input)
		return out, err
	}

	s, doneCh := runActiveStream(ctx, l.pctx, method, req.GetPayload(), invoker)

	// Spawn pumps. Use a derived context so we can cancel the
	// inbound pump (which may be blocked on link.ReadFrame) once
	// the wasm method returns.
	pumpCtx, cancelPumps := context.WithCancel(ctx)
	defer cancelPumps()

	inboundDone := make(chan struct{})
	go func() {
		defer close(inboundDone)
		pumpInboundFrames(pumpCtx, stream, s)
	}()
	outboundDone := make(chan struct{})
	go func() {
		defer close(outboundDone)
		pumpOutboundFrames(pumpCtx, stream, s)
	}()

	// Wait for the wasm method to return.
	res := <-doneCh

	// runActiveStream's defer closes s.outbound which lets the
	// outbound pump emit its terminal EOF + return.
	<-outboundDone

	// Inbound pump may still be blocked on link.ReadFrame. Cancel
	// the derived context so it exits + then close s.inbound for
	// safety (in case the wasm called host_stream_read again
	// after we cleared activeStream — defensive, shouldn't happen).
	cancelPumps()
	<-inboundDone

	if res.err != nil {
		log.L.Warn("plugin.stream.invoke_failed",
			"plugin_id", req.GetPluginId(),
			"stream_name", req.GetStreamName(),
			"method", method,
			"error", res.err.Error(),
		)
		return res.err
	}
	return nil
}

// wireError emits a terminal KIND_ERROR PluginStreamFrame so the
// peer sees a structured failure, then returns the error wrapped
// with the same code so the caller's logs share the vocabulary.
// The write is best-effort — a failure means the wire is already
// broken and there's nothing useful to do.
func wireError(w io.Writer, code, message string) error {
	_ = link.WriteFrame(w, &v2pb.PluginStreamFrame{
		Source:       v2pb.PluginStreamFrame_SOURCE_OUTBOUND,
		Kind:         v2pb.PluginStreamFrame_KIND_ERROR,
		ErrorCode:    code,
		ErrorMessage: message,
	})
	return fmt.Errorf("plugin: %s: %s", code, message)
}
