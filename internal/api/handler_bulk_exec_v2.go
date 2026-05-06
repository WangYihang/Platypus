package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// BulkExecRequest is the JSON body shape for the bulk exec endpoint.
// Same fan-out semantics as BulkPluginCallRequest but routed through
// RpcRequest_Exec — every operator-visible "run this command on N
// hosts" UI hits this single endpoint.
type BulkExecRequest struct {
	AgentIDs       []string          `json:"agent_ids"`
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	CWD            string            `json:"cwd,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	TimeoutMS      uint32            `json:"timeout_ms,omitempty"`
	MaxConcurrency int               `json:"max_concurrency,omitempty"`
}

// BulkExecResult mirrors v2pb.ExecResponse projected onto the
// per-agent row. ok=true means the transport completed and the
// agent reported back — exit_code may still be non-zero (the
// command ran but failed). The error field captures
// transport/offline failures only; service-level command failures
// surface via exit_code + stderr instead.
type BulkExecResult struct {
	AgentID  string `json:"agent_id"`
	Ok       bool   `json:"ok"`
	ExitCode int32  `json:"exit_code,omitempty"`
	Stdout   []byte `json:"stdout,omitempty"`
	Stderr   []byte `json:"stderr,omitempty"`
	Error    string `json:"error,omitempty"`
}

type BulkExecResponse struct {
	Results []BulkExecResult `json:"results"`
}

// v2BulkExec validates the request body, runs the project-pivot
// guard, then fans the ExecRequest out across agent_ids.
//
// @Summary     Bulk command exec across N agents
// @Description Runs the same shell command on every agent_id in parallel.
// @Description ok=true means the agent ran the command and reported back; a
// @Description non-zero exit_code does NOT flip ok to false (that's a successful
// @Description round-trip with a structured "command failed" outcome). Use the
// @Description `error` field to detect transport / agent_offline failures.
// @Tags        bulk-rpc
// @Accept      json
// @Produce     json
// @Param       pid  path string           true "Project ID"
// @Param       body body BulkExecRequest  true "Bulk exec request"
// @Success     200 {object} BulkExecResponse
// @Failure     400 {string} string "invalid body / agent_ids / command"
// @Failure     403 {object} map[string]string "agent not in project"
// @Security    BearerAuth
// @Router      /api/v1/projects/{pid}/agents/bulk/exec [post]
func v2BulkExec(svc *core.AgentLinkService, rbac *RBAC) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req BulkExecRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.String(http.StatusBadRequest, "invalid body: %s", err)
			return
		}
		if len(req.AgentIDs) == 0 {
			c.String(http.StatusBadRequest, "agent_ids must be non-empty")
			return
		}
		if req.Command == "" {
			c.String(http.StatusBadRequest, "command is required")
			return
		}
		if !rbac.checkAgentsInProject(c, req.AgentIDs) {
			return
		}

		dispatcher := func(dctx context.Context, agentID string, rpc *v2pb.RpcRequest) (*v2pb.RpcResponse, error) {
			return core.CallAgentRPC(dctx, svc, agentID, rpc)
		}

		rpcReq := &v2pb.RpcRequest{
			Payload: &v2pb.RpcRequest_Exec{Exec: &v2pb.ExecRequest{
				Command:   req.Command,
				Args:      req.Args,
				Cwd:       req.CWD,
				Env:       req.Env,
				TimeoutMs: req.TimeoutMS,
			}},
		}
		results := core.FanOutRPC(c.Request.Context(), req.AgentIDs, rpcReq,
			core.FanOutOptions{MaxConcurrency: req.MaxConcurrency},
			dispatcher,
		)

		out := BulkExecResponse{
			Results: make([]BulkExecResult, len(results)),
		}
		for i, r := range results {
			row := BulkExecResult{AgentID: r.AgentID}
			switch {
			case r.Err != nil:
				var notConnected *core.ErrAgentNotConnected
				if errors.As(r.Err, &notConnected) {
					row.Error = "agent_offline: " + notConnected.AgentID
				} else {
					row.Error = r.Err.Error()
				}
			case r.Resp == nil:
				row.Error = "no response"
			case r.Resp.GetError() != "":
				row.Error = r.Resp.GetError()
			default:
				ex := r.Resp.GetExec()
				if ex == nil {
					row.Error = "unexpected_response_type"
					break
				}
				if ex.GetError() != "" {
					row.Error = ex.GetError()
				} else {
					row.Ok = true
					row.ExitCode = ex.GetExitCode()
					row.Stdout = ex.GetStdout()
					row.Stderr = ex.GetStderr()
				}
			}
			out.Results[i] = row
		}

		log.L.Info("http.rpc.bulk.exec",
			"project_id", c.Param("pid"),
			"command", req.Command,
			"agents", len(req.AgentIDs),
		)
		c.JSON(http.StatusOK, out)
	}
}
