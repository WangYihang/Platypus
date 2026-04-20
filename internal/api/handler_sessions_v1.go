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

// upgradeRequest is the JSON body of POST /api/v1/sessions/:id/upgrade.
type upgradeRequest struct {
	ListenerID string `json:"listener_id" binding:"required"`
}

// collectAllSessions returns every TCP and Termite client across every
// listener. Mirrors the legacy ListClients loop but returns a slice.
func collectAllSessions() []interface{} {
	out := []interface{}{}
	for _, server := range core.GetServers() {
		for _, c := range server.Clients {
			out = append(out, c)
		}
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
	if client := core.FindTCPClientByHash(hash); client != nil {
		c.JSON(http.StatusOK, client)
		return
	}
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
		if client, ok := server.Clients[hash]; ok {
			core.DeleteTCPClient(client)
			c.Status(http.StatusNoContent)
			return
		}
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
// @Description Executes one shell command and returns its stdout. Plain TCP sessions in PTY mode are refused (409); switch to non-PTY or use the WebSocket terminal. Replaces the legacy form-encoded POST /api/client/{hash}.
// @Tags        sessions
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id   path      string      true "Session hash"
// @Param       body body      execRequest true "Shell command"
// @Success     200  {object}  execResponse
// @Failure     400  {object}  errorResponse
// @Failure     404  {object}  errorResponse
// @Failure     409  {object}  errorResponse
// @Router      /api/v1/sessions/{id}/exec [post]
func ExecSessionV1(c *gin.Context) {
	hash := c.Param("id")
	var req execRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command is required"})
		return
	}
	for _, server := range core.GetServers() {
		if client, ok := server.Clients[hash]; ok {
			if client.GetPtyEstablished() {
				c.JSON(http.StatusConflict, gin.H{"error": "session is under PTY mode; use WebSocket terminal or exit PTY first"})
				return
			}
			c.JSON(http.StatusOK, execResponse{Output: client.SystemToken(req.Command)})
			return
		}
		if client, ok := server.TermiteClients[hash]; ok {
			c.JSON(http.StatusOK, execResponse{Output: client.System(req.Command)})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
}

// UpgradeSessionV1 kicks off a plain→termite upgrade.
//
// @Summary     Upgrade session to Termite
// @Description Compiles a Termite agent for the session's architecture and pushes it over the existing plain reverse shell, where it reconnects to the target encrypted listener. Progress is broadcast on /notify; this endpoint returns 202 as soon as the upgrade is scheduled. Replaces the legacy GET /api/client/{hash}/upgrade/{target}.
// @Tags        sessions
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id   path      string         true "Source session hash (plain TCP client)"
// @Param       body body      upgradeRequest true "Target encrypted listener hash"
// @Success     202  {object}  ackResponse
// @Failure     400  {object}  errorResponse
// @Failure     404  {object}  errorResponse
// @Router      /api/v1/sessions/{id}/upgrade [post]
func UpgradeSessionV1(c *gin.Context) {
	hash := c.Param("id")
	var req upgradeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "listener_id is required"})
		return
	}
	client := core.FindTCPClientByHash(hash)
	if client == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found (upgrade applies to plain TCP sessions)"})
		return
	}
	go client.UpgradeToTermite(req.ListenerID)
	c.JSON(http.StatusAccepted, ackResponse{Status: true, Msg: "upgrade scheduled; watch /notify for progress"})
}
