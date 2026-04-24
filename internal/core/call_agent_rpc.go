package core

import (
	"context"
	"fmt"

	"github.com/WangYihang/Platypus/internal/link"
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
func CallAgentRPC(ctx context.Context, svc *AgentLinkService, agentID string, req *v2pb.RpcRequest) (*v2pb.RpcResponse, error) {
	if svc == nil {
		return nil, fmt.Errorf("core: CallAgentRPC: nil AgentLinkService")
	}
	sess, ok := svc.Get(agentID)
	if !ok {
		return nil, &ErrAgentNotConnected{AgentID: agentID}
	}
	return link.CallRPC(ctx, sess, req)
}
