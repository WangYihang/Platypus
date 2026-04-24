package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// SessionsV2Handler exposes per-project + per-host session queries
// and the project-scoped dispatch route. Dispatch goes through the
// v2 agent link (CallAgentRPC + exec) — no v1 dependency.
type SessionsV2Handler struct {
	db    *storage.DB
	links *core.AgentLinkService
}

func NewSessionsV2Handler(db *storage.DB, links *core.AgentLinkService) *SessionsV2Handler {
	return &SessionsV2Handler{db: db, links: links}
}

type sessionResponse struct {
	ID             string     `json:"id"`
	ProjectID      string     `json:"project_id"`
	IngressAddr    string     `json:"ingress_addr"`
	HostID         string     `json:"host_id"`
	Alias          string     `json:"alias,omitempty"`
	User           string     `json:"user,omitempty"`
	RemoteAddr     string     `json:"remote_addr,omitempty"`
	Version        string     `json:"version,omitempty"`
	GroupDispatch  bool       `json:"group_dispatch"`
	ConnectedAt    time.Time  `json:"connected_at"`
	DisconnectedAt *time.Time `json:"disconnected_at,omitempty"`
}

func toSessionResponse(s *storage.Session) sessionResponse {
	return sessionResponse{
		ID: s.ID, ProjectID: s.ProjectID, IngressAddr: s.IngressAddr, HostID: s.HostID,
		Alias: s.Alias, User: s.User, RemoteAddr: s.RemoteAddr, Version: s.Version,
		GroupDispatch: s.GroupDispatch,
		ConnectedAt:   s.ConnectedAt, DisconnectedAt: s.DisconnectedAt,
	}
}

// ListForHost handles GET /projects/:pid/hosts/:hid/sessions. Returns
// every session (live + historical) for the given host, newest first.
// Host cross-project isolation enforced: the host must belong to :pid.
func (h *SessionsV2Handler) ListForHost(c *gin.Context) {
	pid := c.Param("pid")
	hid := c.Param("hid")
	host, err := h.db.Hosts().GetByID(c.Request.Context(), hid)
	if errors.Is(err, storage.ErrNotFound) || (err == nil && host.ProjectID != pid) {
		c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup host"})
		return
	}
	sessions, err := h.db.Sessions().ListForHost(c.Request.Context(), hid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list sessions"})
		return
	}
	out := make([]sessionResponse, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, toSessionResponse(s))
	}
	c.JSON(http.StatusOK, gin.H{"sessions": out})
}

type dispatchV2Request struct {
	Command string `json:"command" binding:"required"`
	Timeout int    `json:"timeout"` // seconds; defaults to 3
}

type dispatchV2Result struct {
	SessionHash string `json:"session_hash"`
	HostID      string `json:"host_id"`
	Output      string `json:"output"`
	Error       string `json:"error,omitempty"`
}

// Dispatch handles POST /projects/:pid/dispatch. Runs the command against
// every live session in the project whose group_dispatch flag is on,
// returning per-session output (or a timeout error) without giving up
// on the whole batch if one session hangs.
func (h *SessionsV2Handler) Dispatch(c *gin.Context) {
	var req dispatchV2Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command is required"})
		return
	}
	if req.Timeout <= 0 {
		req.Timeout = 3
	}

	live, err := h.db.Sessions().ListLiveForProject(c.Request.Context(), c.Param("pid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list live sessions"})
		return
	}

	// The v1 mirror (core.FindAgentClientByHash + in-memory
	// GroupDispatch flag) is gone; we respect the stored flag only.
	// Agent lookup uses the v2 AgentLinkService keyed by session ID
	// — which in the v2 world IS the agent_id registered at link
	// bring-up.
	results := make([]dispatchV2Result, 0, len(live))
	timeout := time.Duration(req.Timeout) * time.Second
	for _, s := range live {
		if !s.GroupDispatch {
			continue
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		resp, err := core.CallAgentRPC(ctx, h.links, s.ID, &v2pb.RpcRequest{
			Payload: &v2pb.RpcRequest_Exec{Exec: &v2pb.ExecRequest{Command: req.Command}},
		})
		cancel()
		switch {
		case err != nil:
			var notConnected *core.ErrAgentNotConnected
			msg := err.Error()
			if errors.As(err, &notConnected) {
				msg = "session runtime missing"
			}
			results = append(results, dispatchV2Result{
				SessionHash: s.ID, HostID: s.HostID, Error: msg,
			})
		case resp.Error != "":
			results = append(results, dispatchV2Result{
				SessionHash: s.ID, HostID: s.HostID, Error: resp.Error,
			})
		default:
			e := resp.GetExec()
			out := ""
			if e != nil {
				out = string(e.Stdout)
			}
			results = append(results, dispatchV2Result{
				SessionHash: s.ID, HostID: s.HostID, Output: out,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{"count": len(results), "results": results})
}

// ListForProject handles GET /projects/:pid/sessions. Returns sessions
// across the whole project, newest first, with optional filters:
//
//	?live=true    only currently-connected sessions
//	?live=false   only closed sessions
//	?since=ISO    only sessions whose connected_at is >= the timestamp
//	?limit=N      cap results at N rows (after sorting)
//
// Backs SessionsPage and the dashboard time-series chart.
func (h *SessionsV2Handler) ListForProject(c *gin.Context) {
	opts := storage.SessionListOpts{}

	if v, ok := c.GetQuery("live"); ok {
		switch v {
		case "true", "1":
			t := true
			opts.Live = &t
		case "false", "0":
			f := false
			opts.Live = &f
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "live must be true or false"})
			return
		}
	}
	if v, ok := c.GetQuery("since"); ok {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "since must be RFC3339"})
			return
		}
		opts.Since = &t
	}
	if v, ok := c.GetQuery("limit"); ok {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a non-negative integer"})
			return
		}
		opts.Limit = n
	}

	sessions, err := h.db.Sessions().ListForProject(c.Request.Context(), c.Param("pid"), opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list sessions"})
		return
	}
	out := make([]sessionResponse, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, toSessionResponse(s))
	}
	c.JSON(http.StatusOK, gin.H{"sessions": out})
}

// RegisterV1ProjectSessionsRoutes mounts the per-project session routes.
// Host listings are viewer-level; dispatch requires operator because it
// runs code on remote machines.
func RegisterV1ProjectSessionsRoutes(engine *gin.Engine, h *SessionsV2Handler, rbac *RBAC) {
	engine.GET("/api/v1/projects/:pid/sessions",
		rbac.RequireAuth(),
		rbac.RequireProjectRole("pid", user.RoleViewer),
		h.ListForProject,
	)
	engine.GET("/api/v1/projects/:pid/hosts/:hid/sessions",
		rbac.RequireAuth(),
		rbac.RequireProjectRole("pid", user.RoleViewer),
		h.ListForHost,
	)
	engine.POST("/api/v1/projects/:pid/dispatch",
		rbac.RequireAuth(),
		rbac.RequireProjectRole("pid", user.RoleOperator),
		h.Dispatch,
	)
}
