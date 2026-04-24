package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/storage"
)

// v1 session endpoints mirror the legacy /api/client/* routes under a
// JSON-native shape: a consistent {sessions:[]} envelope for lists, the
// bare session object for singles, proper 4xx/5xx codes, and JSON bodies
// on every write. The legacy routes stay alive (now carrying a Deprecation
// header) until clients migrate.

// sessionsListResponse is the shape of GET /api/v1/sessions.
type sessionsListResponse struct {
	Sessions []interface{} `json:"sessions"`
}

// execRequest is the JSON body of POST /api/v1/sessions/:id/exec.
type execRequest struct {
	Command string `json:"command" binding:"required"`
}

// execResponse is the shape of POST /api/v1/sessions/:id/exec.
type execResponse struct {
	Output string `json:"output"`
}

// collectAllSessions returns every connected agent.
func collectAllSessions() []interface{} {
	agents := core.AllAgents()
	out := make([]interface{}, 0, len(agents))
	for _, c := range agents {
		out = append(out, c)
	}
	return out
}

// ListSessionsV1 returns every connected session as a flat list.
//
// @Summary     List sessions
// @Description Returns every connected agent session as a flat array.
// @Tags        sessions
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} sessionsListResponse
// @Router      /api/v1/sessions [get]
func ListSessionsV1(c *gin.Context) {
	c.JSON(http.StatusOK, sessionsListResponse{Sessions: collectAllSessions()})
}

// GetSessionV1 returns one session by hash.
//
// @Summary     Get session
// @Description Looks up a single agent session by hash.
// @Tags        sessions
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      string  true "Session hash"
// @Success     200 {object}  sessionEntry "bare AgentClient JSON"
// @Failure     404 {object}  errorResponse
// @Router      /api/v1/sessions/{id} [get]
func GetSessionV1(c *gin.Context) {
	hash := c.Param("id")
	if client := core.FindAgentClientByHash(hash); client != nil {
		c.JSON(http.StatusOK, client)
		return
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
}

// DeleteSessionV1 disconnects a session.
//
// @Summary     Delete session
// @Description Disconnects the session with the given hash. Returns 204 on success, 404 if no such session. Replaces the legacy DELETE /api/client/{hash}.
// @Tags        sessions
// @Produce     json
// @Security    BearerAuth
// @Param       id  path     string true "Session hash"
// @Success     204 "No Content"
// @Failure     404 {object} errorResponse
// @Router      /api/v1/sessions/{id} [delete]
func DeleteSessionV1(c *gin.Context) {
	hash := c.Param("id")
	if client := core.FindAgentClientByHash(hash); client != nil {
		core.DeleteAgentClient(client)
		c.Status(http.StatusNoContent)
		return
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
}

// ExecSessionV1 runs a shell command on a session and returns its output.
//
// @Summary     Execute command on session
// @Description Executes one shell command on a connected agent and returns its stdout.
// @Tags        sessions
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id   path      string      true "Session hash"
// @Param       body body      execRequest true "Shell command"
// @Success     200  {object}  execResponse
// @Failure     400  {object}  errorResponse
// @Failure     404  {object}  errorResponse
// @Router      /api/v1/sessions/{id}/exec [post]
func ExecSessionV1(c *gin.Context) {
	hash := c.Param("id")
	var req execRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command is required"})
		return
	}
	if client := core.FindAgentClientByHash(hash); client != nil {
		start := time.Now().UTC()
		output := client.System(req.Command)
		dur := time.Since(start).Milliseconds()
		RecordActivity(c, ActivityInput{
			Category:    storage.CategoryCommand,
			Action:      "command.exec",
			TargetType:  "session",
			TargetID:    client.Hash,
			TargetLabel: client.OnelineDesc(),
			SessionID:   client.Hash,
			DurationMs:  &dur,
			At:          start,
			Meta: map[string]any{
				"command":      req.Command,
				"stdout_bytes": len(output),
				"host":         client.Host,
				"remote_addr":  clientAddrOf(client),
			},
		})
		c.JSON(http.StatusOK, execResponse{Output: output})
		return
	}
	// Recorded so a miss on a stale / unknown session shows up in the
	// activity feed — useful for detecting drift between UI and agents.
	RecordActivity(c, ActivityInput{
		Category:    storage.CategoryCommand,
		Action:      "command.exec",
		TargetType:  "session",
		TargetID:    hash,
		TargetLabel: hash,
		SessionID:   hash,
		Outcome:     storage.OutcomeDenied,
		Error:       "session not found",
		Meta:        map[string]any{"command": req.Command},
	})
	c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
}

// clientAddrOf returns the agent's remote address string if the session
// exposes one, else "". The AgentClient API doesn't guarantee a conn
// pointer, so we tolerate nil rather than panicking on disconnected
// sessions.
func clientAddrOf(c *core.AgentClient) string {
	if c == nil {
		return ""
	}
	addr := c.GetConnString()
	return addr
}
