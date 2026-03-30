package api

import (
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/gin-gonic/gin"
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
		v1.PATCH("/sessions/:id", PatchSession)
		v1.POST("/sessions/:id/gather", GatherSession)
		v1.POST("/sessions/dispatch", DispatchCommand)

		// File operations
		v1.GET("/sessions/:id/files", ReadFile)
		v1.POST("/sessions/:id/files", WriteFile)
		v1.GET("/sessions/:id/files/size", GetFileSize)

		// Tunnels
		v1.GET("/sessions/:id/tunnels", ListTunnels)
		v1.POST("/sessions/:id/tunnels", CreateTunnel)
	}
}
