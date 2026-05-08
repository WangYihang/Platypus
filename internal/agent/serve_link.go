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

// AgentHandlerDeps bundles the agent's per-stream handler entry
// points. The wire-level dispatch table is intentionally tiny now:
//
//   - RPC          — STREAM_TYPE_RPC, the unary-RPC carrier
//   - Upgrade      — STREAM_TYPE_AGENT_UPGRADE, agent self-update
//   - PluginMgmt   — STREAM_TYPE_PLUGIN_MGMT, install/uninstall/list
//   - PluginStream — pre-dispatch hook owned by the plugin runtime
//
// Every other stream type (file_read, file_write, file_scan,
// file_archive, process_open) is owned by a system wasm plugin: the
// plugin's manifest claims the stream type, the runtime registers
// the claim with the registry, and PluginStream dispatches the wire
// frames straight into the wasm sandbox. The previous Go-resident
// handlers (HandleFileReadStream et al.) are gone — their
// byte-for-byte wire equivalents now live inside
// examples/plugins/system/sys-{file,process}-* and are delivered to
// the agent via the system-plugin bundle.
type AgentHandlerDeps struct {
	RPC        AgentRPCHandlers
	Upgrade    UpgradeHandler
	PluginMgmt PluginMgmtHandler

	// PluginStream is the dispatch hook the plugin runtime injects.
	// Called before the per-type switch below; when it returns
	// handled=true the switch is skipped. Used by system plugins
	// that have claimed ownership of a stream type via their
	// manifest's `streams:` arm.
	//
	// When nil (the plugin runtime didn't initialise — e.g. unit
	// tests that don't need the wasm path), the type switch handles
	// only the four built-ins above and rejects everything else.
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
//                   built-in switch in dispatchAgentStream (RPC /
//                   upgrade / plugin-mgmt) or end with a reject
type PluginStreamDispatcher func(ctx context.Context, t v2pb.StreamType, stream io.ReadWriteCloser, metadata []byte) (handled bool, err error)

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
