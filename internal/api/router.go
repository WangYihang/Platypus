package api

import (
	"github.com/gin-gonic/gin"
)

// RegisterV1Routes wires the v1 API surface that the v2 rewrite did
// NOT delete: server info, the WebSocket auth-ticket exchange. All
// per-session endpoints (sessions CRUD, file R/W, tunnels, etc.) have
// moved to /api/v1/agents/:agent_id/* (handler_file_v2.go,
// handler_rpc_v2.go, handler_terminal_v2.go). handler_sessions_v2
// still owns /api/v1/projects/:pid/sessions for the admin listing.
func RegisterV1Routes(engine *gin.Engine, auth *Auth) {
	// Public: token endpoint
	engine.POST("/api/v1/auth/token", auth.TokenEndpoint())

	v1 := engine.Group("/api/v1")
	v1.Use(auth.Middleware())
	{
		v1.GET("/info", GetServerInfoV1)

		// WebSocket ticket — browsers trade a Bearer token for a
		// short-lived ?ticket= URL param so they can upgrade
		// /notify without setting Authorization headers.
		v1.POST("/ws/ticket", IssueWSTicket(auth))
	}
}

// IssueWSTicket handles POST /api/v1/ws/ticket.
//
// @Summary     Issue WebSocket ticket
// @Description Mint a one-shot, short-lived ticket (60s TTL) for WebSocket auth. Browsers pass this as ?ticket= on the WS URL because they can't set Bearer headers.
// @Tags        auth
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} wsTicketResponse
// @Router      /api/v1/ws/ticket [post]
func IssueWSTicket(auth *Auth) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, gin.H{"ticket": auth.IssueWSTicket()})
	}
}
