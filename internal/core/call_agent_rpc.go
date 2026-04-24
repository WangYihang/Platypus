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
// Emits one structured log line per call (agent_rpc_start +
// agent_rpc_ok/app_error/failed/not_connected) with a locally
// generated corr_id so the server log alone can be grep'd for a
// full round-trip; the same field set surfaces in the agent-side
// logs (by type + agent_id + approximate timestamp) even though
// we don't wire the corr_id across the link today.
func CallAgentRPC(ctx context.Context, svc *AgentLinkService, agentID string, req *v2pb.RpcRequest) (*v2pb.RpcResponse, error) {
	if svc == nil {
		return nil, fmt.Errorf("core: CallAgentRPC: nil AgentLinkService")
	}
	reqType := rpcPayloadName(req.GetPayload())
	corrID := shortID()
	start := time.Now()
	log.L.Info("agent_rpc_start",
		"agent_id", agentID,
		"type", reqType,
		"corr_id", corrID,
	)

	sess, ok := svc.Get(agentID)
	if !ok {
		log.L.Warn("agent_rpc_not_connected",
			"agent_id", agentID,
			"type", reqType,
			"corr_id", corrID,
			"elapsed_ms", time.Since(start).Milliseconds(),
		)
		return nil, &ErrAgentNotConnected{AgentID: agentID}
	}

	resp, err := link.CallRPC(ctx, sess, req)
	elapsed := time.Since(start).Milliseconds()
	switch {
	case err != nil:
		log.L.Warn("agent_rpc_failed",
			"agent_id", agentID,
			"type", reqType,
			"corr_id", corrID,
			"elapsed_ms", elapsed,
			"error", err.Error(),
		)
	case resp != nil && resp.GetError() != "":
		log.L.Warn("agent_rpc_app_error",
			"agent_id", agentID,
			"type", reqType,
			"corr_id", corrID,
			"elapsed_ms", elapsed,
			"error", resp.GetError(),
		)
	default:
		respType := ""
		if resp != nil {
			respType = rpcPayloadName(resp.GetPayload())
		}
		log.L.Info("agent_rpc_ok",
			"agent_id", agentID,
			"type", reqType,
			"corr_id", corrID,
			"elapsed_ms", elapsed,
			"resp_type", respType,
		)
	}
	return resp, err
}

// rpcPayloadName returns a stable short string for one of the
// oneof variants carried by RpcRequest/RpcResponse. Used for log
// fields only; unknown / nil payloads collapse to sentinel strings
// so log consumers can still filter on them.
func rpcPayloadName(p any) string {
	switch p.(type) {
	case *v2pb.RpcRequest_Exec, *v2pb.RpcResponse_Exec:
		return "exec"
	case *v2pb.RpcRequest_ListDir, *v2pb.RpcResponse_ListDir:
		return "list_dir"
	case *v2pb.RpcRequest_Stat, *v2pb.RpcResponse_Stat:
		return "stat"
	case *v2pb.RpcRequest_Delete, *v2pb.RpcResponse_Delete:
		return "delete"
	case *v2pb.RpcRequest_Rename, *v2pb.RpcResponse_Rename:
		return "rename"
	case *v2pb.RpcRequest_Mkdir, *v2pb.RpcResponse_Mkdir:
		return "mkdir"
	case *v2pb.RpcRequest_Chmod, *v2pb.RpcResponse_Chmod:
		return "chmod"
	case *v2pb.RpcRequest_SysInfo, *v2pb.RpcResponse_SysInfo:
		return "sys_info"
	case *v2pb.RpcRequest_ProcessList, *v2pb.RpcResponse_ProcessList:
		return "process_list"
	case nil:
		return ""
	default:
		return "unknown"
	}
}

// shortID returns 8 hex chars of crypto/rand for log correlation.
// Not a wire value — never leaves the server.
func shortID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b[:])
}
