package plugin

import (
	"context"
	"errors"
	"io"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// pumpInboundFrames reads PluginStreamFrame messages from the wire
// and feeds the byte payload of every INBOUND DATA frame onto
// s.inbound. KIND_EOF closes s.inbound + flips the inboundEOF flag
// so subsequent host_stream_read calls return empty data without
// blocking. Wire-level read errors (peer hangup, ctx cancel) also
// close the channel — the wasm reader treats both EOF paths
// identically.
//
// OUTBOUND-source frames received from the wire are a protocol
// violation (server is supposed to send INBOUND only); the pump
// silently drops them rather than corrupting the wasm reader's
// view. Errors are logged at the dispatcher level — this function
// stays a tight loop with no per-frame allocation beyond what
// link.ReadFrame already does.
//
// Returns when the wire closes or ctx cancels. Idempotent close
// of s.inbound: a second call to close() would panic, but the
// guard via inboundEOF.Swap(true) prevents it.
func pumpInboundFrames(ctx context.Context, r io.Reader, s *streamCtx) {
	defer func() {
		if !s.inboundEOF.Swap(true) {
			close(s.inbound)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		var frame v2pb.PluginStreamFrame
		if err := link.ReadFrame(r, &frame); err != nil {
			return // EOF / peer close / parse error → close channel via defer
		}
		if frame.GetSource() != v2pb.PluginStreamFrame_SOURCE_INBOUND {
			continue // server bug; tolerate by skipping
		}
		switch frame.GetKind() {
		case v2pb.PluginStreamFrame_KIND_DATA:
			select {
			case s.inbound <- frame.GetData():
			case <-ctx.Done():
				return
			}
		case v2pb.PluginStreamFrame_KIND_EOF, v2pb.PluginStreamFrame_KIND_ERROR:
			return // close channel via defer
		}
	}
}

// pumpOutboundFrames drains s.outbound and writes one OUTBOUND DATA
// frame per chunk. When s.outbound is closed (host_stream_close
// fired or runActiveStream cleaned up) emits a terminal OUTBOUND
// EOF frame so the peer knows the stream is done.
//
// Wire-level write errors stop the pump (peer hung up; nothing more
// we can do). Returns when s.outbound is closed AND the EOF frame
// is written.
func pumpOutboundFrames(ctx context.Context, w io.Writer, s *streamCtx) {
	for {
		select {
		case b, ok := <-s.outbound:
			if !ok {
				_ = link.WriteFrame(w, &v2pb.PluginStreamFrame{
					Source: v2pb.PluginStreamFrame_SOURCE_OUTBOUND,
					Kind:   v2pb.PluginStreamFrame_KIND_EOF,
				})
				return
			}
			if err := link.WriteFrame(w, &v2pb.PluginStreamFrame{
				Source: v2pb.PluginStreamFrame_SOURCE_OUTBOUND,
				Kind:   v2pb.PluginStreamFrame_KIND_DATA,
				Data:   b,
			}); err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// errPumpClosed is the sentinel pump callers check for when they
// want to distinguish "wire ended cleanly" from a real error. Not
// used today (the dispatcher logs at the layer above) but reserved
// so a future code path that needs the distinction has a stable
// symbol to compare against.
var errPumpClosed = errors.New("plugin: pump closed cleanly")
