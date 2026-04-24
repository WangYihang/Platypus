package api

import (
	"github.com/gin-gonic/gin"
)

// RegisterV1Routes registers all /api/v1/ routes with authentication.
// Call this after CreateRESTfulAPIServer() to add the new endpoints.
func RegisterV1Routes(engine *gin.Engine, auth *Auth) {
	// Public: token endpoint
	engine.POST("/api/v1/auth/token", auth.TokenEndpoint())

	// Protected routes
	v1 := engine.Group("/api/v1")
	v1.Use(auth.Middleware())
	{
		// Server info
		v1.GET("/info", GetServerInfoV1)

		// Sessions
		v1.GET("/sessions", ListSessionsV1)
		v1.GET("/sessions/:id", GetSessionV1)
		v1.DELETE("/sessions/:id", DeleteSessionV1)
		v1.PATCH("/sessions/:id", PatchSession)
		v1.POST("/sessions/:id/exec", ExecSessionV1)
		v1.POST("/sessions/:id/gather", GatherSession)
		v1.POST("/sessions/dispatch", DispatchCommand)

		// File operations
		v1.GET("/sessions/:id/files", ReadFile)
		v1.POST("/sessions/:id/files", WriteFile)
		v1.DELETE("/sessions/:id/files", DeleteFileHandler)
		v1.GET("/sessions/:id/files/size", GetFileSize)
		v1.GET("/sessions/:id/files/list", ListDirHandler)
		v1.GET("/sessions/:id/files/stat", StatHandler)
		v1.POST("/sessions/:id/files/rename", RenameFileHandler)
		v1.POST("/sessions/:id/files/mkdir", MkdirHandler)
		v1.POST("/sessions/:id/files/chmod", ChmodHandler)

		// Tunnels
		v1.GET("/sessions/:id/tunnels", ListTunnels)
		v1.POST("/sessions/:id/tunnels", CreateTunnel)

		// WebSocket ticket issue — trades the Bearer token for a one-shot
		// short-lived ticket that browsers can pass via ?ticket= when
		// upgrading /ws/:hash or /notify.
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
