package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// ErrAgentNotConnected is returned by CallAgentRPC when the
// supplied agent_id has no live Session registered in the
// AgentLinkService. REST handlers propagate this as a 404 / 503.
type ErrAgentNotConnected struct{ AgentID string }

func (e *ErrAgentNotConnected) Error() string {
	return fmt.Sprintf("core: no live v2 session for agent %q", e.AgentID)
}

// CallAgentRPC is the one-call helper REST handlers use to invoke
// an RPC on a v2-connected agent. It looks up the Session in svc
// and delegates to link.CallRPC; if the agent is not connected, it
// returns a typed error so callers can map it to the right HTTP
// status without string-matching.
//
// Each call emits one structured log line per outcome under the
// `rpc.call.*` namespace:
//
//	rpc.call.start          -> always, before dispatch
//	rpc.call.ok             -> handler ran cleanly
//	rpc.call.app_error      -> handler ran but reported a service error
//	rpc.call.transport_error-> link / yamux / framing failure
//	rpc.call.agent_offline  -> no live session for agent_id
//
// Every line carries the same context envelope: agent_id,
// project_id (when known), link_session_id, correlation_id,
// rpc_method, plus a `request` slog.Group with the relevant
// payload fields. Success and failure lines additionally carry
// elapsed_ms and a `response` group (ok path only).
func CallAgentRPC(ctx context.Context, svc *AgentLinkService, agentID string, req *v2pb.RpcRequest) (*v2pb.RpcResponse, error) {
	if svc == nil {
		return nil, fmt.Errorf("core: CallAgentRPC: nil AgentLinkService")
	}
	method := log.RPCMethodName(req.GetPayload())
	correlationID := newCorrelationID()
	requestAttr := log.RPCRequestAttr(req.GetPayload())
	start := time.Now()

	sess, linkSessionID, ok := svc.GetWithSessionID(agentID)
	baseFields := []any{
		"agent_id", agentID,
		"link_session_id", linkSessionID,
		"correlation_id", correlationID,
		"rpc_method", method,
		requestAttr,
	}
	log.L.Info("rpc.call.start", baseFields...)

	if !ok {
		log.L.Warn("rpc.call.agent_offline", append(baseFields,
			"elapsed_ms", time.Since(start).Milliseconds(),
		)...)
		return nil, &ErrAgentNotConnected{AgentID: agentID}
	}

	resp, err := link.CallRPC(ctx, sess, req, correlationID)
	elapsed := time.Since(start).Milliseconds()
	switch {
	case err != nil:
		log.L.Warn("rpc.call.transport_error", append(baseFields,
			"elapsed_ms", elapsed,
			"error", err.Error(),
		)...)
	case resp != nil && resp.GetError() != "":
		log.L.Warn("rpc.call.app_error", append(baseFields,
			"elapsed_ms", elapsed,
			"error", resp.GetError(),
		)...)
	default:
		log.L.Info("rpc.call.ok", append(baseFields,
			"elapsed_ms", elapsed,
			log.RPCResponseAttr(resp.GetPayload()),
		)...)
	}
	return resp, err
}

// newCorrelationID returns 8 hex chars of crypto/rand for log
// correlation. Travels in StreamHeader.correlation_id so the agent
// echoes the same id and a single grep can pull both sides of one
// round-trip out of the log stream. Not a wire authentication value.
func newCorrelationID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b[:])
}
