package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
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

// collectAllSessions returns every Termite agent session across every listener.
func collectAllSessions() []interface{} {
	out := []interface{}{}
	for _, server := range core.GetServers() {
		for _, c := range server.TermiteClients {
			out = append(out, c)
		}
	}
	return out
}

// ListSessionsV1 returns every connected session as a flat list.
//
// @Summary     List sessions
// @Description Returns every connected session (plain TCP + encrypted Termite) as a flat array. Replaces the legacy /api/client, which stays available but now sends Deprecation headers.
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
// @Description Looks up a single session by hash (plain TCP or Termite). Replaces the legacy /api/client/{hash}.
// @Tags        sessions
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      string  true "Session hash"
// @Success     200 {object}  legacyClientEntry "bare TCPClient or TermiteClient JSON"
// @Failure     404 {object}  errorResponse
// @Router      /api/v1/sessions/{id} [get]
func GetSessionV1(c *gin.Context) {
	hash := c.Param("id")
	if client := core.FindTermiteClientByHash(hash); client != nil {
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
	for _, server := range core.GetServers() {
		if client, ok := server.TermiteClients[hash]; ok {
			core.DeleteTermiteClient(client)
			c.Status(http.StatusNoContent)
			return
		}
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
	for _, server := range core.GetServers() {
		if client, ok := server.TermiteClients[hash]; ok {
			c.JSON(http.StatusOK, execResponse{Output: client.System(req.Command)})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
}

