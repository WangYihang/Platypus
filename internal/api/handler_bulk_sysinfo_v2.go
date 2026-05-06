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

// BulkSysInfoRequest fans the SysInfo RPC across N agents. The
// request itself is intentionally minimal — sys_info has no fields,
// fan-out is the only knob the operator tunes.
type BulkSysInfoRequest struct {
	AgentIDs       []string `json:"agent_ids"`
	TimeoutMS      uint32   `json:"timeout_ms,omitempty"`
	MaxConcurrency int      `json:"max_concurrency,omitempty"`
}

// BulkSysInfoResult projects the SysInfoResponse fields the fleet
// rollup UI cares about. We deliberately don't dump all 100+
// SysInfoResponse fields per row — bulk callers asking for "show
// me hostnames + memory + load across the fleet" get a 5x smaller
// wire payload than embedding the full struct.
//
// The single-agent /sys endpoint stays the right entry point for
// the rich detail view; this is the rollup-summary shape.
type BulkSysInfoResult struct {
	AgentID       string  `json:"agent_id"`
	Ok            bool    `json:"ok"`
	Hostname      string  `json:"hostname,omitempty"`
	Os            string  `json:"os,omitempty"`
	Arch          string  `json:"arch,omitempty"`
	KernelVersion string  `json:"kernel_version,omitempty"`
	NumCpu        uint32  `json:"num_cpu,omitempty"`
	MemTotal      uint64  `json:"mem_total,omitempty"`
	MemUsed       uint64  `json:"mem_used,omitempty"`
	UptimeSeconds uint64  `json:"uptime_seconds,omitempty"`
	Load1         float64 `json:"load1,omitempty"`
	ProcessCount  uint32  `json:"process_count,omitempty"`
	Error         string  `json:"error,omitempty"`
}

type BulkSysInfoResponse struct {
	Results []BulkSysInfoResult `json:"results"`
}

func v2BulkSysInfo(svc *core.AgentLinkService, rbac *RBAC) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req BulkSysInfoRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.String(http.StatusBadRequest, "invalid body: %s", err)
			return
		}
		if len(req.AgentIDs) == 0 {
			c.String(http.StatusBadRequest, "agent_ids must be non-empty")
			return
		}
		if !rbac.checkAgentsInProject(c, req.AgentIDs) {
			return
		}

		dispatcher := func(dctx context.Context, agentID string, rpc *v2pb.RpcRequest) (*v2pb.RpcResponse, error) {
			return core.CallAgentRPC(dctx, svc, agentID, rpc)
		}

		rpcReq := &v2pb.RpcRequest{
			Payload: &v2pb.RpcRequest_SysInfo{SysInfo: &v2pb.SysInfoRequest{}},
		}
		results := core.FanOutRPC(c.Request.Context(), req.AgentIDs, rpcReq,
			core.FanOutOptions{MaxConcurrency: req.MaxConcurrency},
			dispatcher,
		)

		out := BulkSysInfoResponse{
			Results: make([]BulkSysInfoResult, len(results)),
		}
		for i, r := range results {
			row := BulkSysInfoResult{AgentID: r.AgentID}
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
				si := r.Resp.GetSysInfo()
				if si == nil {
					row.Error = "unexpected_response_type"
					break
				}
				if si.GetError() != "" {
					row.Error = si.GetError()
				} else {
					row.Ok = true
					row.Hostname = si.GetHostname()
					row.Os = si.GetOs()
					row.Arch = si.GetArch()
					row.KernelVersion = si.GetKernelVersion()
					row.NumCpu = si.GetNumCpu()
					row.MemTotal = si.GetMemTotal()
					row.MemUsed = si.GetMemUsed()
					row.UptimeSeconds = si.GetUptimeSeconds()
					row.Load1 = si.GetLoad1()
					row.ProcessCount = si.GetProcessCount()
				}
			}
			out.Results[i] = row
		}

		log.L.Info("http.rpc.bulk.sys_info",
			"project_id", c.Param("pid"),
			"agents", len(req.AgentIDs),
		)
		c.JSON(http.StatusOK, out)
	}
}
