package api

import (
	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/log"
)

// RegisterV1Routes registers all /api/v1/ routes with authentication.
// Call this after CreateRESTfulAPIServer() to add the new endpoints.
func RegisterV1Routes(engine *gin.Engine, auth *Auth) {
	log.Info("Registering /api/v1/ routes with Bearer Token authentication")

	// Public: token endpoint
	engine.POST("/api/v1/auth/token", auth.TokenEndpoint())

	// Protected routes
	v1 := engine.Group("/api/v1")
	v1.Use(auth.Middleware())
	{
		// Sessions
		v1.GET("/sessions", ListSessionsV1)
		v1.GET("/sessions/:id", GetSessionV1)
		v1.DELETE("/sessions/:id", DeleteSessionV1)
		v1.PATCH("/sessions/:id", PatchSession)
		v1.POST("/sessions/:id/exec", ExecSessionV1)
		v1.POST("/sessions/:id/upgrade", UpgradeSessionV1)
		v1.POST("/sessions/:id/gather", GatherSession)
		v1.POST("/sessions/dispatch", DispatchCommand)

		// File operations
		v1.GET("/sessions/:id/files", ReadFile)
		v1.POST("/sessions/:id/files", WriteFile)
		v1.GET("/sessions/:id/files/size", GetFileSize)

		// Tunnels
		v1.GET("/sessions/:id/tunnels", ListTunnels)
		v1.POST("/sessions/:id/tunnels", CreateTunnel)

		// RaaS — single source of truth for the one-liner templates. Desktop
		// and web clients call these rather than shipping their own copies
		// of internal/utils/raas/templates/.
		v1.GET("/raas/languages", ListRaasLanguages)
		v1.GET("/raas/oneliner", RenderRaasOneliner)

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
