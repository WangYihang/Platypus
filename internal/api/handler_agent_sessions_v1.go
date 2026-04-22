package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// AgentSessionsHandler exposes the admin-only operations on an agent's
// session_token. Currently just Revoke (kill the active session), which
// forces the agent to either re-enroll with its original PAT (if still
// valid) or be cut off entirely. The view/list endpoints can be added
// later without changing this surface.
type AgentSessionsHandler struct {
	db *storage.DB
}

func NewAgentSessionsHandler(db *storage.DB) *AgentSessionsHandler {
	return &AgentSessionsHandler{db: db}
}

type revokeSessionRequest struct {
	Reason string `json:"reason"`
}

// Revoke handles POST /api/v1/agents/:aid/sessions/revoke.
//
// Semantics: marks whatever session is currently active for the
// agent_id as revoked. All append-only rows (history, events) remain
// intact. The agent keeps its current TLS connection alive until its
// next renewal attempt or reconnect — both will fail and the agent
// will then exit / redial and hit the PAT-or-session-file path.
//
// We don't proactively kill the in-memory AgentClient TCP handshake
// here; doing so would require cross-package plumbing into internal/core
// and the primary kill-switch is still the DB state. A follow-up can
// signal the dispatcher to disconnect immediately.
func (h *AgentSessionsHandler) Revoke(c *gin.Context) {
	agentID := c.Param("aid")
	claims, _ := ClaimsFromContext(c)

	var req revokeSessionRequest
	// Body is optional — missing or malformed JSON just means "no reason".
	_ = c.ShouldBindJSON(&req)

	err := h.db.AgentSessions().RevokeActive(c.Request.Context(), agentID, claims.UserID, req.Reason, time.Now().UTC())
	if errors.Is(err, storage.ErrNotFound) {
		h.audit(c, "session.revoke", "agent_session", agentID, "", req, "denied", "no active session")
		c.JSON(http.StatusNotFound, gin.H{"error": "no active session"})
		return
	}
	if err != nil {
		h.audit(c, "session.revoke", "agent_session", agentID, "", req, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "revoke session"})
		return
	}
	h.audit(c, "session.revoke", "agent_session", agentID, "", req, "success", "")
	c.Status(http.StatusNoContent)
}

// History handles GET /api/v1/agents/:aid/sessions. Returns every
// session generation the agent has ever had, newest first. Useful for
// audit: who rotated when, who revoked, etc. Plaintext tokens are
// never returned (they're not stored).
func (h *AgentSessionsHandler) History(c *gin.Context) {
	agentID := c.Param("aid")
	sessions, err := h.db.AgentSessions().History(c.Request.Context(), agentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "history"})
		return
	}
	items := make([]map[string]any, 0, len(sessions))
	for _, s := range sessions {
		active := s.IsActive(time.Now())
		items = append(items, map[string]any{
			"session_id":      s.SessionID,
			"agent_id":        s.AgentID,
			"project_id":      s.ProjectID,
			"issued_at":       s.IssuedAt,
			"issued_reason":   s.IssuedReason,
			"rotated_from":    s.RotatedFrom,
			"expires_at":      s.ExpiresAt,
			"rotated_at":      s.RotatedAt,
			"revoked_at":      s.RevokedAt,
			"revoked_reason":  s.RevokedReason,
			"revoked_by_user": s.RevokedByUser,
			"last_seen_at":    s.LastSeenAt,
			"last_seen_ip":    s.LastSeenIP,
			"machine_id":      s.MachineID,
			"active":          active,
		})
	}
	c.JSON(http.StatusOK, gin.H{"sessions": items})
}

// audit is the same small helper PAT / install handlers use. Kept
// local to avoid cross-handler plumbing.
func (h *AgentSessionsHandler) audit(c *gin.Context, action, targetType, targetID, projectID string, details interface{}, outcome, errText string) {
	claims, _ := ClaimsFromContext(c)
	blob, _ := json.Marshal(details)
	_ = h.db.AdminAuditLog().Record(c.Request.Context(), &storage.AdminAuditEvent{
		At:         time.Now().UTC(),
		ActorUser:  claims.UserID,
		ActorIP:    c.ClientIP(),
		ActorUA:    c.Request.UserAgent(),
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		ProjectID:  projectID,
		Details:    string(blob),
		Outcome:    outcome,
		Error:      errText,
	})
}

// RegisterV1AgentSessionsRoutes mounts the session-admin endpoints.
// Not scoped to a project because agent_id is global; the auth gate is
// global-admin only. Tighter per-agent RBAC can come later once the
// project ↔ agent edge table exists.
func RegisterV1AgentSessionsRoutes(engine *gin.Engine, h *AgentSessionsHandler, rbac *RBAC) {
	grp := engine.Group("/api/v1/agents/:aid/sessions")
	grp.Use(rbac.RequireAuth(), rbac.RequireGlobalRole(user.RoleAdmin))
	{
		grp.GET("", h.History)
		grp.POST("/revoke", h.Revoke)
	}
}
