package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/ipinfo"
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
	ID             string       `json:"id"`
	ProjectID      string       `json:"project_id"`
	IngressAddr    string       `json:"ingress_addr"`
	HostID         string       `json:"host_id"`
	Alias          string       `json:"alias,omitempty"`
	User           string       `json:"user,omitempty"`
	RemoteAddr     string       `json:"remote_addr,omitempty"`
	RemoteInfo     *ipinfo.Info `json:"remote_info,omitempty"`
	Version        string       `json:"version,omitempty"`
	GroupDispatch  bool         `json:"group_dispatch"`
	ConnectedAt    time.Time    `json:"connected_at"`
	DisconnectedAt *time.Time   `json:"disconnected_at,omitempty"`
}

func toSessionResponse(s *storage.Session) sessionResponse {
	resp := sessionResponse{
		ID: s.ID, ProjectID: s.ProjectID, IngressAddr: s.IngressAddr, HostID: s.HostID,
		Alias: s.Alias, User: s.User, RemoteAddr: s.RemoteAddr, Version: s.Version,
		GroupDispatch: s.GroupDispatch,
		ConnectedAt:   s.ConnectedAt, DisconnectedAt: s.DisconnectedAt,
	}
	if s.RemoteAddr != "" {
		info := ipinfo.Lookup(s.RemoteAddr)
		resp.RemoteInfo = &info
	}
	return resp
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
//
// "Live" here is decided by core.AgentLinkService — the in-memory
// registry of currently-registered agent links. The DB rows from
// ListLiveForProject (rows with disconnected_at IS NULL) are an
// audit-tail filter only; a row whose host's agent isn't in the
// registry right now is a stale audit window (crash + reboot before
// the next sweep) and gets dropped silently rather than producing a
// "session runtime missing" error row that would just confuse
// operators.
func (h *SessionsV2Handler) Dispatch(c *gin.Context) {
	var req dispatchV2Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command is required"})
		return
	}
	if req.Timeout <= 0 {
		req.Timeout = 3
	}

	dispatchStart := time.Now().UTC()

	openRows, err := h.db.Sessions().ListLiveForProject(c.Request.Context(), c.Param("pid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list sessions"})
		return
	}
	liveAgents := liveAgentSet(h.links)

	results := make([]dispatchV2Result, 0, len(openRows))
	timeout := time.Duration(req.Timeout) * time.Second
	var errCount int
	for _, s := range openRows {
		if !s.GroupDispatch {
			continue
		}
		agentID, ok := h.resolveLiveAgentForSession(c.Request.Context(), s, liveAgents)
		if !ok {
			// Audit-tail row whose agent isn't actually live — drop
			// it. A real "agent went down between filter and call"
			// race shows up below as ErrAgentNotConnected and gets
			// surfaced as a session-runtime-missing error row.
			continue
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		resp, err := core.CallAgentRPC(ctx, h.links, agentID, &v2pb.RpcRequest{
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
			errCount++
		case resp.Error != "":
			results = append(results, dispatchV2Result{
				SessionHash: s.ID, HostID: s.HostID, Error: resp.Error,
			})
			errCount++
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

	// One audit row covers the whole fan-out. Per-session error
	// detail lives in the response body and the per-RPC core logs;
	// the activity row summarises so dashboards can answer "who ran
	// what against how many hosts?" with a single query.
	dispatchDur := time.Since(dispatchStart).Milliseconds()
	outcome := storage.OutcomeSuccess
	if len(results) > 0 && errCount == len(results) {
		outcome = storage.OutcomeError
	}
	RecordActivity(c, ActivityInput{
		Category:   storage.CategoryCommand,
		Action:     "command.dispatch",
		TargetType: "project",
		TargetID:   c.Param("pid"),
		Outcome:    outcome,
		DurationMs: &dispatchDur,
		At:         dispatchStart,
		Meta: map[string]any{
			"command":      truncateForAudit(req.Command, 256),
			"timeout_s":    req.Timeout,
			"dispatched":   len(results),
			"errors":       errCount,
			"live_agents":  len(liveAgents),
		},
	})
}

// liveAgentSet snapshots AgentLinkService into a set the handler can
// O(1) probe per row. Cheap because AgentLinkService.IDs() already
// returns a defensive slice; we just reshape it.
func liveAgentSet(links *core.AgentLinkService) map[string]struct{} {
	ids := links.IDs()
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}

// resolveLiveAgentForSession returns the agent_id behind a session
// row's host, but only if that agent is currently registered with
// AgentLinkService. Returns ("", false) when the host has no agent_id
// yet (transient enroll race) or the agent is not in the live set
// (audit-tail row).
func (h *SessionsV2Handler) resolveLiveAgentForSession(ctx context.Context, s *storage.Session, liveAgents map[string]struct{}) (string, bool) {
	host, err := h.db.Hosts().GetByID(ctx, s.HostID)
	if err != nil || host.AgentID == "" {
		return "", false
	}
	if _, ok := liveAgents[host.AgentID]; !ok {
		return "", false
	}
	return host.AgentID, true
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

	// SSOT presence intersection: when the caller asked for "live",
	// every returned row must correspond to an agent that is right
	// now registered in core.AgentLinkService. The DB filter
	// (disconnected_at IS NULL) is a coarse audit-window pre-filter
	// — it can return rows the previous server instance left open
	// after a SIGKILL. AgentLinkService is the only source of truth
	// for "this link is alive", so we drop any row whose host's
	// agent isn't in it.
	if opts.Live != nil && *opts.Live {
		live := liveAgentSet(h.links)
		filtered := sessions[:0]
		for _, s := range sessions {
			if _, ok := h.resolveLiveAgentForSession(c.Request.Context(), s, live); ok {
				filtered = append(filtered, s)
			}
		}
		sessions = filtered
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
