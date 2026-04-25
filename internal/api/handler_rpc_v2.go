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
	"github.com/WangYihang/Platypus/internal/user"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

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
	)
	operator.DELETE("/fs/remove", logRPCHandler("delete", v2RPCDelete(svc)))
	operator.POST("/fs/rename", logRPCHandler("rename", v2RPCRename(svc)))
	operator.POST("/fs/mkdir", logRPCHandler("mkdir", v2RPCMkdir(svc)))
	operator.PATCH("/fs/mode", logRPCHandler("chmod", v2RPCChmod(svc)))
	operator.POST("/exec", logRPCHandler("exec", v2RPCExec(svc)))
}

// logRPCHandler wraps a per-endpoint gin handler with enter / exit
// structured logs so each v2 agent-RPC HTTP call is traceable
// without touching each body. The CallAgentRPC round-trip itself
// is logged independently in internal/core, so these two layers
// together show: handler wallclock vs. RPC wallclock vs. middleware
// overhead.
func logRPCHandler(name string, fn gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		log.L.Info("http_rpc_enter",
			"name", name,
			"agent_id", c.Param("agent_id"),
			"path", c.Request.URL.Path,
		)
		fn(c)
		log.L.Info("http_rpc_exit",
			"name", name,
			"agent_id", c.Param("agent_id"),
			"status", c.Writer.Status(),
			"elapsed_ms", time.Since(start).Milliseconds(),
		)
	}
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
		c.JSON(http.StatusOK, gin.H{"entries": ld.Entries})
	}
}

func v2RPCStat(svc *core.AgentLinkService) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp := callOrAbort(c, svc, &v2pb.RpcRequest{
			Payload: &v2pb.RpcRequest_Stat{Stat: &v2pb.StatRequest{Path: c.Query("path")}},
		})
		if resp == nil {
			return
		}
		s := resp.GetStat()
		if s.Error != "" {
			c.String(http.StatusBadGateway, "%s", s.Error)
			return
		}
		c.JSON(http.StatusOK, gin.H{"entry": s.Entry})
	}
}

func v2RPCDelete(svc *core.AgentLinkService) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp := callOrAbort(c, svc, &v2pb.RpcRequest{
			Payload: &v2pb.RpcRequest_Delete{Delete: &v2pb.DeleteRequest{
				Path:      c.Query("path"),
				Recursive: c.Query("recursive") == "true",
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
		resp := callOrAbort(c, svc, &v2pb.RpcRequest{
			Payload: &v2pb.RpcRequest_Rename{Rename: &v2pb.RenameRequest{
				From: c.Query("from"), To: c.Query("to"),
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
		resp := callOrAbort(c, svc, &v2pb.RpcRequest{
			Payload: &v2pb.RpcRequest_Mkdir{Mkdir: &v2pb.MkdirRequest{
				Path:   c.Query("path"),
				Mode:   uint32(modeU64),
				Mkdirs: c.Query("mkdirs") == "true",
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
		resp := callOrAbort(c, svc, &v2pb.RpcRequest{
			Payload: &v2pb.RpcRequest_Chmod{Chmod: &v2pb.ChmodRequest{
				Path: c.Query("path"), Mode: uint32(modeU64),
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
		c.JSON(http.StatusOK, gin.H{
			"stdout":    string(e.Stdout),
			"stderr":    string(e.Stderr),
			"exit_code": e.ExitCode,
			"error":     e.Error,
		})
	}
}
