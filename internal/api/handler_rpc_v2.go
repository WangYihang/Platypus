package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// activityMetaKey is the gin.Context key per-RPC handlers use to
// stash a meta map for the audit row written by logRPCHandler. Kept
// untyped (a plain string) because gin's context store is a
// map[string]any and a typed key would require an unexported helper
// in every caller.
const activityMetaKey = "rpc.activity_meta"

// rpcAuditMeta is the (category, action) pair logRPCHandler stamps on
// the activity row for each RPC method. Defined here so the route
// table at the top of the file stays a one-liner per endpoint.
type rpcAuditMeta struct {
	category string
	action   string
}

// rpcAuditTable maps the symbolic method name passed to
// logRPCHandler to the audit category/action recorded for that
// endpoint. Keeping this in one place means a new RPC method only
// has to (a) add a route, (b) add an entry here.
var rpcAuditTable = map[string]rpcAuditMeta{
	"list_dir": {storage.CategoryFile, "file.list"},
	"stat":     {storage.CategoryFile, "file.stat"},
	"delete":   {storage.CategoryFile, "file.delete"},
	"rename":   {storage.CategoryFile, "file.rename"},
	"mkdir":    {storage.CategoryFile, "file.mkdir"},
	"chmod":    {storage.CategoryFile, "file.chmod"},
	"sys_info": {storage.CategoryAgent, "agent.sysinfo"},
	"exec":     {storage.CategoryCommand, "command.exec"},
}

// RegisterV2AgentRPCRoutes wires the one-shot RPC endpoints into
// the Gin engine. Each route is a thin adapter: parse query/body
// into the matching RpcRequest oneof, call core.CallAgentRPC,
// serialise the RpcResponse back as JSON.
//
// Semantics are uniform across the set:
//   - Missing agent id   → 404
//   - Agent-reported err → 502 with the error text
//   - RPC not implemented by this agent build → 501 with "unsupported"
//
// URL shapes are project-scoped: every endpoint is mounted under
// /api/v1/projects/:pid/agents/:agent_id/... so the existing
// project-RBAC middleware (RequireProjectRole) enforces who may even
// reach the agent. RequireAgentInProject closes the cross-project
// pivot vector by verifying the agent actually belongs to :pid.
//
// The viewer/operator split mirrors the action's blast radius: read
// ops (list, stat, sys) require viewer; mutations (delete, rename,
// mkdir, chmod, exec) require operator.
func RegisterV2AgentRPCRoutes(engine *gin.Engine, svc *core.AgentLinkService, rbac *RBAC) {
	base := engine.Group("/api/v1/projects/:pid/agents/:agent_id")
	base.Use(rbac.RequireAuth())

	viewer := base.Group("")
	viewer.Use(
		rbac.RequireProjectRole("pid", user.RoleViewer),
		rbac.RequireAgentInProject("pid", "agent_id"),
	)
	viewer.GET("/fs/list", logRPCHandler("list_dir", v2RPCListDir(svc)))
	viewer.GET("/fs/stat", logRPCHandler("stat", v2RPCStat(svc)))
	viewer.GET("/sys", logRPCHandler("sys_info", v2RPCSysInfo(svc)))

	operator := base.Group("")
	operator.Use(
		rbac.RequireProjectRole("pid", user.RoleOperator),
		rbac.RequireAgentInProject("pid", "agent_id"),
		// Per-principal token-bucket rate limit. A compromised AAT or
		// session can't drive unbounded shell-exec / file-mutation
		// churn against every host in the project. Default budget
		// (30 burst, 10/s refill) is set in rpc_rate.go and lets
		// normal interactive use through.
		rbac.RequireRPCRateLimit(),
	)
	operator.DELETE("/fs/remove", logRPCHandler("delete", v2RPCDelete(svc)))
	operator.POST("/fs/rename", logRPCHandler("rename", v2RPCRename(svc)))
	operator.POST("/fs/mkdir", logRPCHandler("mkdir", v2RPCMkdir(svc)))
	operator.PATCH("/fs/mode", logRPCHandler("chmod", v2RPCChmod(svc)))
	operator.POST("/exec", logRPCHandler("exec", v2RPCExec(svc)))
}

// logRPCHandler wraps a per-endpoint gin handler with start / finish
// structured logs so each v2 agent-RPC HTTP call is traceable
// without touching each body. The CallAgentRPC round-trip itself
// is logged independently in internal/core; these two layers together
// show: handler wallclock vs. RPC wallclock vs. middleware overhead.
//
// `http.rpc.start` and `http.rpc.finish` carry agent_id, project_id,
// rpc_method and the canonical HTTP fields (`http_method`,
// `http_path`, `client_ip`). Naming intentionally avoids `path`
// because that key is also used by RPC payloads (e.g. list_dir.path);
// `http_path` keeps them disjoint.
func logRPCHandler(method string, fn gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		baseFields := []any{
			"rpc_method", method,
			"agent_id", c.Param("agent_id"),
			"project_id", c.Param("pid"),
			"http_method", c.Request.Method,
			"http_path", c.Request.URL.Path,
			"client_ip", c.ClientIP(),
		}
		log.L.Info("http.rpc.start", baseFields...)
		fn(c)
		status := c.Writer.Status()
		elapsed := time.Since(start)
		log.L.Info("http.rpc.finish", append(baseFields,
			"http_status", status,
			"elapsed_ms", elapsed.Milliseconds(),
		)...)
		recordRPCActivity(c, method, start, elapsed, status)
	}
}

// recordRPCActivity emits the audit row for one RPC HTTP call. Pulls
// the per-handler meta map (path, mode, command, …) the handler
// stashed via c.Set(activityMetaKey, …); a missing meta map is fine
// — the row still records the action and target. Outcome is
// inferred from the gin response status: 2xx → success, otherwise →
// error. The error message comes from the body the handler wrote,
// surfaced through c.Errors when one was appended; otherwise we fall
// back to the status text.
func recordRPCActivity(c *gin.Context, method string, start time.Time, elapsed time.Duration, status int) {
	meta := rpcAuditTable[method]
	if meta.action == "" {
		// Unknown method — skip the audit row rather than emit a
		// half-typed event. Keeping the symbol table the source of
		// truth means a typo at the call site is loud (no audit row)
		// instead of a silent miscategorisation.
		return
	}

	durMs := elapsed.Milliseconds()
	in := ActivityInput{
		Category:   meta.category,
		Action:     meta.action,
		TargetType: "agent",
		TargetID:   c.Param("agent_id"),
		DurationMs: &durMs,
		At:         start.UTC(),
	}
	if v, ok := c.Get(activityMetaKey); ok {
		in.Meta = v
	}
	if status >= 200 && status < 300 {
		in.Outcome = storage.OutcomeSuccess
	} else {
		in.Outcome = storage.OutcomeError
		// Surface whatever the handler tried to communicate; falls
		// back to a generic status note when nothing was attached.
		if len(c.Errors) > 0 {
			in.Error = c.Errors.String()
		} else {
			in.Error = http.StatusText(status)
		}
	}
	RecordActivity(c, in)
}

// callOrAbort is the shared plumbing: lookup agent, call the RPC,
// map errors to HTTP status. Returns the RpcResponse for the
// caller to unwrap; any non-nil returned *gin.Context-reply means
// the caller already wrote the response and should just return.
func callOrAbort(c *gin.Context, svc *core.AgentLinkService, req *v2pb.RpcRequest) *v2pb.RpcResponse {
	agentID := c.Param("agent_id")
	resp, err := core.CallAgentRPC(c.Request.Context(), svc, agentID, req)
	if err != nil {
		var notConnected *core.ErrAgentNotConnected
		if errors.As(err, &notConnected) {
			c.String(http.StatusNotFound, "agent %s not connected", agentID)
		} else {
			c.String(http.StatusBadGateway, "agent call: %s", err)
		}
		return nil
	}
	// Service-level error (handler registered but failed to run).
	if resp.Error != "" {
		// "not supported" classification → 501. Everything else is
		// treated as an upstream failure (502).
		if strings.Contains(resp.Error, "not supported") || strings.Contains(resp.Error, "unsupported") {
			c.String(http.StatusNotImplemented, "%s", resp.Error)
		} else {
			c.String(http.StatusBadGateway, "%s", resp.Error)
		}
		return nil
	}
	return resp
}

func v2RPCListDir(svc *core.AgentLinkService) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Query("path")
		c.Set(activityMetaKey, map[string]any{"path": path})
		resp := callOrAbort(c, svc, &v2pb.RpcRequest{
			Payload: &v2pb.RpcRequest_ListDir{ListDir: &v2pb.ListDirRequest{Path: path}},
		})
		if resp == nil {
			return
		}
		ld := resp.GetListDir()
		if ld.Error != "" {
			c.String(http.StatusBadGateway, "%s", ld.Error)
			return
		}
		c.JSON(http.StatusOK, gin.H{"entries": EnrichFileEntries(ld.Entries)})
	}
}

func v2RPCStat(svc *core.AgentLinkService) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Query("path")
		c.Set(activityMetaKey, map[string]any{"path": path})
		resp := callOrAbort(c, svc, &v2pb.RpcRequest{
			Payload: &v2pb.RpcRequest_Stat{Stat: &v2pb.StatRequest{Path: path}},
		})
		if resp == nil {
			return
		}
		s := resp.GetStat()
		if s.Error != "" {
			c.String(http.StatusBadGateway, "%s", s.Error)
			return
		}
		c.JSON(http.StatusOK, gin.H{"entry": EnrichFileEntry(s.Entry)})
	}
}

func v2RPCDelete(svc *core.AgentLinkService) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Query("path")
		recursive := c.Query("recursive") == "true"
		c.Set(activityMetaKey, map[string]any{"path": path, "recursive": recursive})
		resp := callOrAbort(c, svc, &v2pb.RpcRequest{
			Payload: &v2pb.RpcRequest_Delete{Delete: &v2pb.DeleteRequest{
				Path:      path,
				Recursive: recursive,
			}},
		})
		if resp == nil {
			return
		}
		d := resp.GetDelete()
		if d.Error != "" {
			c.String(http.StatusBadGateway, "%s", d.Error)
			return
		}
		c.Status(http.StatusOK)
	}
}

func v2RPCRename(svc *core.AgentLinkService) gin.HandlerFunc {
	return func(c *gin.Context) {
		from := c.Query("from")
		to := c.Query("to")
		c.Set(activityMetaKey, map[string]any{"from": from, "to": to})
		resp := callOrAbort(c, svc, &v2pb.RpcRequest{
			Payload: &v2pb.RpcRequest_Rename{Rename: &v2pb.RenameRequest{
				From: from, To: to,
			}},
		})
		if resp == nil {
			return
		}
		if r := resp.GetRename(); r.Error != "" {
			c.String(http.StatusBadGateway, "%s", r.Error)
			return
		}
		c.Status(http.StatusOK)
	}
}

func v2RPCMkdir(svc *core.AgentLinkService) gin.HandlerFunc {
	return func(c *gin.Context) {
		modeU64, _ := strconv.ParseUint(c.Query("mode"), 10, 32)
		path := c.Query("path")
		mkdirs := c.Query("mkdirs") == "true"
		c.Set(activityMetaKey, map[string]any{
			"path":   path,
			"mode":   uint32(modeU64),
			"mkdirs": mkdirs,
		})
		resp := callOrAbort(c, svc, &v2pb.RpcRequest{
			Payload: &v2pb.RpcRequest_Mkdir{Mkdir: &v2pb.MkdirRequest{
				Path:   path,
				Mode:   uint32(modeU64),
				Mkdirs: mkdirs,
			}},
		})
		if resp == nil {
			return
		}
		if m := resp.GetMkdir(); m.Error != "" {
			c.String(http.StatusBadGateway, "%s", m.Error)
			return
		}
		c.Status(http.StatusOK)
	}
}

func v2RPCChmod(svc *core.AgentLinkService) gin.HandlerFunc {
	return func(c *gin.Context) {
		modeU64, err := strconv.ParseUint(c.Query("mode"), 10, 32)
		if err != nil {
			c.String(http.StatusBadRequest, "mode must be numeric")
			return
		}
		path := c.Query("path")
		c.Set(activityMetaKey, map[string]any{"path": path, "mode": uint32(modeU64)})
		resp := callOrAbort(c, svc, &v2pb.RpcRequest{
			Payload: &v2pb.RpcRequest_Chmod{Chmod: &v2pb.ChmodRequest{
				Path: path, Mode: uint32(modeU64),
			}},
		})
		if resp == nil {
			return
		}
		if ch := resp.GetChmod(); ch.Error != "" {
			c.String(http.StatusBadGateway, "%s", ch.Error)
			return
		}
		c.Status(http.StatusOK)
	}
}

func v2RPCSysInfo(svc *core.AgentLinkService) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp := callOrAbort(c, svc, &v2pb.RpcRequest{
			Payload: &v2pb.RpcRequest_SysInfo{SysInfo: &v2pb.SysInfoRequest{}},
		})
		if resp == nil {
			return
		}
		c.JSON(http.StatusOK, resp.GetSysInfo())
	}
}

// v2RPCExecBody is the JSON request shape for POST /exec. Kept
// separate from ExecRequest so the wire layer is JSON rather than
// protobuf for this public endpoint.
type v2RPCExecBody struct {
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Cwd       string            `json:"cwd"`
	Env       map[string]string `json:"env"`
	TimeoutMs uint32            `json:"timeout_ms"`
}

func v2RPCExec(svc *core.AgentLinkService) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 64*1024))
		if err != nil {
			c.String(http.StatusBadRequest, "read body: %s", err)
			return
		}
		var in v2RPCExecBody
		if err := json.Unmarshal(body, &in); err != nil {
			c.String(http.StatusBadRequest, "parse body: %s", err)
			return
		}
		if in.Command == "" {
			c.String(http.StatusBadRequest, "command required")
			return
		}
		// Stash the meta first so the audit row reflects what the
		// user attempted, even if the RPC layer aborts. Args go in
		// truncated form so a "rg foo /" with thousands of paths
		// doesn't bloat the activity row; full args land in
		// structured logs at the agent.
		c.Set(activityMetaKey, map[string]any{
			"command": truncateForAudit(in.Command, 256),
			"args":    truncateArgsForAudit(in.Args, 16, 64),
			"cwd":     in.Cwd,
		})
		resp := callOrAbort(c, svc, &v2pb.RpcRequest{
			Payload: &v2pb.RpcRequest_Exec{Exec: &v2pb.ExecRequest{
				Command:   in.Command,
				Args:      in.Args,
				Cwd:       in.Cwd,
				Env:       in.Env,
				TimeoutMs: in.TimeoutMs,
			}},
		})
		if resp == nil {
			return
		}
		e := resp.GetExec()
		// Augment the meta with the agent-reported result so the
		// audit row distinguishes "exec ran, exit 0" from "exec ran,
		// exit 137".
		c.Set(activityMetaKey, map[string]any{
			"command":   truncateForAudit(in.Command, 256),
			"args":      truncateArgsForAudit(in.Args, 16, 64),
			"cwd":       in.Cwd,
			"exit_code": e.ExitCode,
			"agent_err": e.Error,
		})
		c.JSON(http.StatusOK, gin.H{
			"stdout":    string(e.Stdout),
			"stderr":    string(e.Stderr),
			"exit_code": e.ExitCode,
			"error":     e.Error,
		})
	}
}

// truncateForAudit clips a string to n runes with an ellipsis when
// it overflows. Used to keep the activities.meta column bounded
// regardless of caller-supplied payload size.
func truncateForAudit(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// truncateArgsForAudit caps the slice at maxArgs entries and each
// entry to maxLen runes. Audit rows summarise; full fidelity lives
// in the agent-side execution log.
func truncateArgsForAudit(args []string, maxArgs, maxLen int) []string {
	if len(args) > maxArgs {
		args = args[:maxArgs]
	}
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = truncateForAudit(a, maxLen)
	}
	return out
}
