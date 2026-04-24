package agent

import (
	"context"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// AgentHandlerDeps bundles the per-stream-type handlers the agent
// exposes. Only populated fields are dispatched; unhandled stream
// types get a StreamReject back to the server so dashboards can
// surface capability gaps explicitly.
//
// Process is optional: when nil, STREAM_TYPE_PROCESS_OPEN streams
// get rejected. Tunnel / file / event / socks5 slots will land
// alongside their respective handlers.
type AgentHandlerDeps struct {
	RPC        AgentRPCHandlers
	Process    ProcessHandler
	FileRead   FileReadHandler
	FileWrite  FileWriteHandler
	TunnelPull TunnelPullHandler
}

// ProcessHandler processes one STREAM_TYPE_PROCESS_OPEN stream.
// The production implementation is HandleProcessStream; tests can
// substitute a stub.
type ProcessHandler func(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.ProcessOpenRequest) error

// FileReadHandler processes one STREAM_TYPE_FILE_READ stream.
// Production impl: HandleFileReadStream.
type FileReadHandler func(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.FileReadRequest) error

// FileWriteHandler processes one STREAM_TYPE_FILE_WRITE stream.
// Production impl: HandleFileWriteStream.
type FileWriteHandler func(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.FileWriteRequest) error

// TunnelPullHandler processes one STREAM_TYPE_TUNNEL_PULL stream.
// Production impl: HandleTunnelPullStream.
type TunnelPullHandler func(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.TunnelPullRequest) error

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
			// Operator-initiated shutdown (Ctrl+C / SIGTERM) is the
			// only "we meant for this to happen" path — exit cleanly
			// so the CLI returns 0 and any process supervisor doesn't
			// flap-restart us.
			if ctx.Err() != nil {
				return nil
			}
			// Anything else (server hangup → io.EOF, yamux session
			// teardown, network blip) is an unintended drop. Return a
			// non-nil error so the BackoffRetry loop in main.go
			// reconnects with exponential backoff + jitter rather
			// than treating it as success and exiting the process.
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
	case v2pb.StreamType_STREAM_TYPE_PROCESS_OPEN:
		if deps.Process == nil {
			rejectStream(stream, "unsupported_type", "process handler not registered")
			return
		}
		var req v2pb.ProcessOpenRequest
		if err := proto.Unmarshal(hdr.Metadata, &req); err != nil {
			rejectStream(stream, "malformed_metadata", "parse ProcessOpenRequest: "+err.Error())
			return
		}
		if err := deps.Process(ctx, stream, &req); err != nil {
			log.Warn("agent: process stream for %s: %v", hdr.CorrelationId, err)
		}
	case v2pb.StreamType_STREAM_TYPE_FILE_READ:
		if deps.FileRead == nil {
			rejectStream(stream, "unsupported_type", "file-read handler not registered")
			return
		}
		var req v2pb.FileReadRequest
		if err := proto.Unmarshal(hdr.Metadata, &req); err != nil {
			rejectStream(stream, "malformed_metadata", "parse FileReadRequest: "+err.Error())
			return
		}
		if err := deps.FileRead(ctx, stream, &req); err != nil {
			log.Warn("agent: file-read stream for %s: %v", hdr.CorrelationId, err)
		}
	case v2pb.StreamType_STREAM_TYPE_FILE_WRITE:
		if deps.FileWrite == nil {
			rejectStream(stream, "unsupported_type", "file-write handler not registered")
			return
		}
		var req v2pb.FileWriteRequest
		if err := proto.Unmarshal(hdr.Metadata, &req); err != nil {
			rejectStream(stream, "malformed_metadata", "parse FileWriteRequest: "+err.Error())
			return
		}
		if err := deps.FileWrite(ctx, stream, &req); err != nil {
			log.Warn("agent: file-write stream for %s: %v", hdr.CorrelationId, err)
		}
	case v2pb.StreamType_STREAM_TYPE_TUNNEL_PULL:
		if deps.TunnelPull == nil {
			rejectStream(stream, "unsupported_type", "tunnel-pull handler not registered")
			return
		}
		var req v2pb.TunnelPullRequest
		if err := proto.Unmarshal(hdr.Metadata, &req); err != nil {
			rejectStream(stream, "malformed_metadata", "parse TunnelPullRequest: "+err.Error())
			return
		}
		if err := deps.TunnelPull(ctx, stream, &req); err != nil {
			log.Warn("agent: tunnel-pull stream for %s: %v", hdr.CorrelationId, err)
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
