package agent

import (
	"context"
	"fmt"
	"io"
	"sync"

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
	RPC         AgentRPCHandlers
	Process     ProcessHandler
	FileRead    FileReadHandler
	FileWrite   FileWriteHandler
	FileScan    FileScanHandler
	FileArchive FileArchiveHandler
	TunnelPull  TunnelPullHandler
	Upgrade     UpgradeHandler
	PluginMgmt  PluginMgmtHandler

	// PluginStream is the optional pre-dispatch hook the plugin
	// runtime injects. Called before the legacy hardcoded type
	// switch; when it returns handled=true the legacy switch is
	// skipped. Used by system plugins that have claimed ownership of
	// a stream type via their manifest's `streams:` arm.
	//
	// When nil (e.g. plugin runtime unavailable at boot), the legacy
	// switch handles every stream as it always did.
	PluginStream PluginStreamDispatcher
}

// PluginStreamDispatcher is the signature for the
// AgentHandlerDeps.PluginStream hook. Implementations live in
// internal/agent/plugin (Registry.DispatchStream); declared here so
// the agent package doesn't need to import the plugin package.
//
// Contract:
//   - (true, nil)   plugin owned + ran the stream successfully
//   - (true, err)   plugin owned the stream but the provider errored
//   - (false, nil)  no plugin claimed this type → fall through to the
//                   legacy switch in dispatchAgentStream
type PluginStreamDispatcher func(ctx context.Context, t v2pb.StreamType, stream io.ReadWriteCloser, metadata []byte) (handled bool, err error)

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

// FileScanHandler processes one STREAM_TYPE_FILE_SCAN stream.
// Production impl: HandleFileScanStream.
type FileScanHandler func(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.FileScanRequest) error

// FileArchiveHandler processes one STREAM_TYPE_FILE_ARCHIVE stream.
// Production impl: HandleFileArchiveStream.
type FileArchiveHandler func(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.FileArchiveRequest) error

// TunnelPullHandler processes one STREAM_TYPE_TUNNEL_PULL stream.
// Production impl: HandleTunnelPullStream.
type TunnelPullHandler func(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.TunnelPullRequest) error

// UpgradeHandler processes one STREAM_TYPE_AGENT_UPGRADE stream.
// Terminal: when the install phase succeeds the handler emits a final
// PHASE_EXITING progress frame and then deliberately terminates the
// process so the supervisor restarts it under the new binary; the
// handler therefore never returns from a successful upgrade. Errors
// are reported on-stream as PHASE_FAILED frames and surface here as
// non-nil returns for the dispatch loop to log.
//
// Production impl: (*UpgradeRunner).Handle in upgrade_stream.go.
type UpgradeHandler func(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.AgentUpgradeRequest) error

// PluginMgmtHandler processes one STREAM_TYPE_PLUGIN_MGMT stream:
// install / uninstall / list / enable / get_logs. The dispatcher inside
// the handler picks the variant from req.op. Production impl:
// (*plugin.Registry).HandleMgmt in internal/agent/plugin/mgmt_stream.go.
type PluginMgmtHandler func(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.PluginMgmtRequest) error

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

	// Join in-flight per-stream handlers before returning so the
	// reconnect loop in cmd/platypus-agent doesn't race a fresh
	// Bootstrap/Serve cycle against goroutines from the previous
	// session that are still in synchronous cleanup (fsync on a
	// file write, kill+wait on a process). Without this, repeated
	// reconnects on a flapping link compound goroutines over time.
	var wg sync.WaitGroup
	defer wg.Wait()

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
		wg.Add(1)
		go func() {
			defer wg.Done()
			dispatchAgentStream(ctx, hdr, stream, deps)
		}()
	}
}

// dispatchAgentStream picks the handler for hdr.Type. Unknown
// types get a StreamReject frame before we close — lets the peer
// distinguish "agent version doesn't support this" from an
// outright crash.
func dispatchAgentStream(ctx context.Context, hdr *v2pb.StreamHeader, stream io.ReadWriteCloser, deps AgentHandlerDeps) {
	log.L.Debug("link.stream_open",
		"stream_type", hdr.GetType().String(),
		"correlation_id", hdr.GetCorrelationId(),
		"link_session_id", hdr.GetLinkSessionId(),
	)
	// Seed both ids into ctx so per-RPC log lines and any sub-spans
	// they emit (process_list.*, file_*, etc.) carry the same values
	// the server stamped on the wire.
	ctx = ContextWithStreamIDs(ctx, hdr.GetCorrelationId(), hdr.GetLinkSessionId())

	// Plugin claim consultation: a system plugin may have declared
	// ownership of this stream type via its manifest's `streams:`
	// arm. When it has + the host-side provider is available, the
	// plugin handles the stream and we skip the legacy switch.
	// (false, nil) means no plugin owned this type — fall through.
	if deps.PluginStream != nil {
		handled, err := deps.PluginStream(ctx, hdr.Type, stream, hdr.Metadata)
		if handled {
			if err != nil {
				log.Warn("agent: plugin-claimed stream %s for %s: %v",
					hdr.Type.String(), hdr.CorrelationId, err)
			}
			return
		}
	}

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
	case v2pb.StreamType_STREAM_TYPE_FILE_SCAN:
		if deps.FileScan == nil {
			rejectStream(stream, "unsupported_type", "file-scan handler not registered")
			return
		}
		var req v2pb.FileScanRequest
		if err := proto.Unmarshal(hdr.Metadata, &req); err != nil {
			rejectStream(stream, "malformed_metadata", "parse FileScanRequest: "+err.Error())
			return
		}
		if err := deps.FileScan(ctx, stream, &req); err != nil {
			log.Warn("agent: file-scan stream for %s: %v", hdr.CorrelationId, err)
		}
	case v2pb.StreamType_STREAM_TYPE_FILE_ARCHIVE:
		if deps.FileArchive == nil {
			rejectStream(stream, "unsupported_type", "file-archive handler not registered")
			return
		}
		var req v2pb.FileArchiveRequest
		if err := proto.Unmarshal(hdr.Metadata, &req); err != nil {
			rejectStream(stream, "malformed_metadata", "parse FileArchiveRequest: "+err.Error())
			return
		}
		if err := deps.FileArchive(ctx, stream, &req); err != nil {
			log.Warn("agent: file-archive stream for %s: %v", hdr.CorrelationId, err)
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
	case v2pb.StreamType_STREAM_TYPE_AGENT_UPGRADE:
		if deps.Upgrade == nil {
			rejectStream(stream, "unsupported_type", "upgrade handler not registered")
			return
		}
		var req v2pb.AgentUpgradeRequest
		if err := proto.Unmarshal(hdr.Metadata, &req); err != nil {
			rejectStream(stream, "malformed_metadata", "parse AgentUpgradeRequest: "+err.Error())
			return
		}
		if err := deps.Upgrade(ctx, stream, &req); err != nil {
			log.Warn("agent: upgrade stream for %s: %v", hdr.CorrelationId, err)
		}
	case v2pb.StreamType_STREAM_TYPE_PLUGIN_MGMT:
		if deps.PluginMgmt == nil {
			rejectStream(stream, "unsupported_type", "plugin-mgmt handler not registered")
			return
		}
		var req v2pb.PluginMgmtRequest
		if err := proto.Unmarshal(hdr.Metadata, &req); err != nil {
			rejectStream(stream, "malformed_metadata", "parse PluginMgmtRequest: "+err.Error())
			return
		}
		if err := deps.PluginMgmt(ctx, stream, &req); err != nil {
			log.Warn("agent: plugin-mgmt stream for %s: %v", hdr.CorrelationId, err)
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
