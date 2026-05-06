package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/user"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// BulkPluginCallRequest is the JSON body shape of the bulk plugin
// invoke endpoint. Each agent in agent_ids gets the SAME plugin_call
// dispatched in parallel; the response carries one row per input
// agent in input order.
//
// payload is the opaque plugin-defined request bytes (typically JSON
// the plugin's wasm parses, but free-form). Encoded as base64 in JSON
// per Go's default []byte encoding.
type BulkPluginCallRequest struct {
	AgentIDs       []string `json:"agent_ids"`
	PluginID       string   `json:"plugin_id"`
	Method         string   `json:"method"`
	Payload        []byte   `json:"payload,omitempty"`
	TimeoutMS      uint32   `json:"timeout_ms,omitempty"`
	MaxConcurrency int      `json:"max_concurrency,omitempty"`
}

// BulkPluginCallResult is the per-agent outcome. Exactly one of
// payload / error carries content. ok is the convenience flag
// callers test against (true == payload populated, no error). The
// error string is human-readable; machine-readable categorisation
// (offline / transport / app-error) is reserved for a follow-up if
// the operator UI needs to differentiate.
type BulkPluginCallResult struct {
	AgentID string `json:"agent_id"`
	Ok      bool   `json:"ok"`
	Payload []byte `json:"payload,omitempty"`
	Error   string `json:"error,omitempty"`
}

type BulkPluginCallResponse struct {
	Results []BulkPluginCallResult `json:"results"`
}

// RegisterV2BulkRPCRoutes wires the bulk-RPC endpoints onto the
// project-scoped agent router. Each endpoint requires the operator
// role (writes are observable side effects on N agents) and the
// per-principal RPC rate limiter (single request → N agent calls,
// so the burst budget bites in bulk too).
func RegisterV2BulkRPCRoutes(engine *gin.Engine, svc *core.AgentLinkService, rbac *RBAC) {
	base := engine.Group("/api/v1/projects/:pid/agents/bulk")
	base.Use(rbac.RequireAuth())
	base.Use(
		rbac.RequireProjectRole("pid", user.RoleOperator),
		rbac.RequireRPCRateLimit(),
	)
	base.POST("/plugin_call", v2BulkPluginCall(svc, rbac))
	base.POST("/exec", v2BulkExec(svc, rbac))
	base.POST("/sys_info", v2BulkSysInfo(svc, rbac))
}

// v2BulkPluginCall validates the request, checks every agent is in
// the project (cross-project pivot guard), then fans the call out
// via core.FanOutRPC. Per-agent failures are isolated; the HTTP
// response is 200 even when every result row carries an error —
// the caller is expected to scan the results table.
//
// @Summary     Bulk plugin RPC across N agents
// @Description Dispatches the same plugin_call to every agent_id in parallel.
// @Description Per-agent failures (offline / app error) appear in the corresponding
// @Description result row's `error` field; the HTTP status is 200 unless the request
// @Description body itself was malformed (400) or one of the listed agents is not in
// @Description the project (403).
// @Tags        bulk-rpc
// @Accept      json
// @Produce     json
// @Param       pid  path string                  true "Project ID"
// @Param       body body BulkPluginCallRequest   true "Bulk plugin call request"
// @Success     200 {object} BulkPluginCallResponse
// @Failure     400 {string} string "invalid body / agent_ids / plugin_id / method"
// @Failure     403 {object} map[string]string "agent not in project"
// @Security    BearerAuth
// @Router      /api/v1/projects/{pid}/agents/bulk/plugin_call [post]
func v2BulkPluginCall(svc *core.AgentLinkService, rbac *RBAC) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req BulkPluginCallRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.String(http.StatusBadRequest, "invalid body: %s", err)
			return
		}
		if len(req.AgentIDs) == 0 {
			c.String(http.StatusBadRequest, "agent_ids must be non-empty")
			return
		}
		if req.PluginID == "" {
			c.String(http.StatusBadRequest, "plugin_id is required")
			return
		}
		if req.Method == "" {
			c.String(http.StatusBadRequest, "method is required")
			return
		}

		// Cross-project pivot guard: every agent_id must belong to
		// :pid. Single batched query — looking up N hosts one-by-one
		// would re-acquire the connection N times for a large
		// fleet. The handler-side check intentionally duplicates the
		// per-agent gate the singleton routes get from
		// RequireAgentInProject — there's no convenient way to make
		// gin middleware iterate a list.
		if !rbac.checkAgentsInProject(c, req.AgentIDs) {
			return
		}

		dispatcher := func(dctx context.Context, agentID string, rpc *v2pb.RpcRequest) (*v2pb.RpcResponse, error) {
			return core.CallAgentRPC(dctx, svc, agentID, rpc)
		}

		rpcReq := &v2pb.RpcRequest{
			Payload: &v2pb.RpcRequest_PluginCall{PluginCall: &v2pb.PluginCallRequest{
				PluginId:  req.PluginID,
				Method:    req.Method,
				Payload:   req.Payload,
				TimeoutMs: req.TimeoutMS,
			}},
		}
		results := core.FanOutRPC(c.Request.Context(), req.AgentIDs, rpcReq,
			core.FanOutOptions{MaxConcurrency: req.MaxConcurrency},
			dispatcher,
		)

		out := BulkPluginCallResponse{
			Results: make([]BulkPluginCallResult, len(results)),
		}
		for i, r := range results {
			row := BulkPluginCallResult{AgentID: r.AgentID}
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
				pc := r.Resp.GetPluginCall()
				if pc == nil {
					row.Error = "unexpected_response_type"
					break
				}
				if pc.GetError() != "" {
					row.Error = pc.GetError()
				} else {
					row.Ok = true
					row.Payload = pc.GetPayload()
				}
			}
			out.Results[i] = row
		}

		log.L.Info("http.rpc.bulk.plugin_call",
			"project_id", c.Param("pid"),
			"plugin_id", req.PluginID,
			"method", req.Method,
			"agents", len(req.AgentIDs),
		)
		c.JSON(http.StatusOK, out)
	}
}

