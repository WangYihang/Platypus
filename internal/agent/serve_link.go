package agent

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// AgentHandlerDeps bundles the per-stream-type handlers the agent
// exposes. Only populated fields are dispatched; unhandled stream
// types get a StreamReject back to the server so dashboards can
// surface capability gaps explicitly.
//
// Only RPC is wired right now; PTY / tunnel / file / event / socks5
// slots will be added as those handlers land.
type AgentHandlerDeps struct {
	RPC AgentRPCHandlers
}

// ServeLink is the agent-side accept loop. For each incoming yamux
// stream it reads the StreamHeader (already done by sess.Accept),
// routes by Type, and invokes the appropriate per-type handler on
// its own goroutine so a slow handler can't starve the loop.
//
// Returns nil on clean session close; non-nil on a session-level
// error that should terminate the agent (or at least trigger a
// reconnect).
func ServeLink(ctx context.Context, sess *link.Session, deps AgentHandlerDeps) error {
	// Close the session when ctx cancels so the blocked Accept
	// returns; normal peer-initiated close unwinds through
	// io.EOF below.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = sess.Close()
		case <-done:
		}
	}()

	for {
		hdr, stream, err := sess.Accept()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			// Own-Close raced the Accept; treat as clean.
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("agent: ServeLink accept: %w", err)
		}
		go dispatchAgentStream(ctx, hdr, stream, deps)
	}
}

// dispatchAgentStream picks the handler for hdr.Type. Unknown
// types get a StreamReject frame before we close — lets the peer
// distinguish "agent version doesn't support this" from an
// outright crash.
func dispatchAgentStream(ctx context.Context, hdr *v2pb.StreamHeader, stream io.ReadWriteCloser, deps AgentHandlerDeps) {
	switch hdr.Type {
	case v2pb.StreamType_STREAM_TYPE_RPC:
		if err := ServeRPCStream(ctx, stream, deps.RPC); err != nil {
			log.Warn("agent: RPC stream for %s: %v", hdr.CorrelationId, err)
		}
	default:
		rejectStream(stream, "unsupported_type", fmt.Sprintf("no handler for %s", hdr.Type))
	}
}

// rejectStream writes a StreamReject frame and closes. Best-effort:
// errors writing the frame are logged but not propagated; the peer
// will observe an abrupt close either way.
func rejectStream(stream io.ReadWriteCloser, code, message string) {
	defer func() { _ = stream.Close() }()
	rej := &v2pb.StreamReject{Code: code, Message: message}
	if err := link.WriteFrame(stream, rej); err != nil {
		log.Debug("agent: reject frame write: %v", err)
	}
}
