package plugin

import "context"

// invokerFn is the wasm-call seam runActiveStream injects. Production
// wraps extism.Plugin.CallWithContext on a *loaded; tests substitute a
// closure that fakes wasm by reading + writing through pctx.activeStream.
// Splitting the seam keeps runActiveStream pure-state-machine and lets
// the unit tests run without an extism runtime.
type invokerFn func(ctx context.Context, method string, input []byte) ([]byte, error)

// invokerResult bundles what an invoker returns so runActiveStream can
// hand both fields back through a single channel.
type invokerResult struct {
	output []byte
	err    error
}

// runActiveStream is the choreography around one wasm-streaming
// method call. Allocates a fresh streamCtx, installs it as
// pctx.activeStream so the host_stream_* primitives can find it,
// invokes the wasm method on a goroutine, then cleans up: clears
// activeStream + closes the outbound channel (idempotent — if the
// plugin already called host_stream_close, the close has happened).
//
// Returns immediately (the wasm call runs in the spawned goroutine)
// with:
//   - *streamCtx  the per-stream channels — caller drives byte
//                 plumbing on these
//   - <-chan invokerResult  signals when the wasm method returned;
//                 read for the invoker's (output, err)
//
// The caller (the agent's per-stream-type dispatcher in
// serve_link.go) is responsible for:
//   - reading wire frames + pushing bytes onto s.inbound
//   - draining s.outbound + writing wire frames
//   - closing s.inbound when the wire side EOFs (signals wasm
//     read returns empty)
//
// This split keeps runActiveStream's responsibility tiny + means
// it has no wire-protocol coupling — making it usable for any
// future stream type (file_read, file_write, process_open, ...)
// just by writing a per-type dispatcher on top.
func runActiveStream(ctx context.Context, pctx *pluginCtx, method string, payload []byte, invoker invokerFn) (*streamCtx, <-chan invokerResult) {
	s := &streamCtx{
		inbound:  make(chan []byte, 1),
		outbound: make(chan []byte, 1),
	}
	pctx.setActiveStream(s)

	resultCh := make(chan invokerResult, 1)
	go func() {
		defer pctx.clearActiveStream()
		defer func() {
			// Idempotent close so the dispatcher's outbound drain
			// goroutine unblocks even when a plugin returned without
			// calling host_stream_close (e.g. early-error path).
			if !s.writeClosed.Swap(true) {
				close(s.outbound)
			}
		}()
		out, err := invoker(ctx, method, payload)
		resultCh <- invokerResult{output: out, err: err}
	}()
	return s, resultCh
}
