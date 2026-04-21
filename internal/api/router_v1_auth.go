package api

import (
	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/user"
)

// RegisterV1AuthRoutes mounts the JWT-based auth and user-management
// endpoints on the supplied engine. Kept as a sibling to RegisterV1Routes
// rather than folded into it so that (a) the legacy shared-secret
// middleware still guards the older protected routes untouched, and
// (b) tests can mount just the new surface when they don't need the
// legacy ones.
func RegisterV1AuthRoutes(engine *gin.Engine, auth *AuthHandler, users *UsersHandler, rbac *RBAC) {
	authGroup := engine.Group("/api/v1/auth")
	authGroup.POST("/bootstrap", auth.Bootstrap)
	authGroup.POST("/login", auth.Login)
	authGroup.POST("/refresh", auth.Refresh)
	authGroup.POST("/logout", auth.Logout)

	// Self-service password change requires the caller to already be
	// authenticated — the handler's old_password check on top is a
	// second line of defence, not the primary gate.
	authedGroup := engine.Group("/api/v1/auth")
	authedGroup.Use(rbac.RequireAuth())
	authedGroup.PATCH("/password", auth.ChangePassword)

	adminOnly := engine.Group("/api/v1/users")
	adminOnly.Use(rbac.RequireAuth(), rbac.RequireGlobalRole(user.RoleAdmin))
	{
		adminOnly.GET("", users.List)
		adminOnly.POST("", users.Create)
		adminOnly.GET("/:id", users.Get)
		adminOnly.PATCH("/:id", users.Update)
		adminOnly.DELETE("/:id", users.Delete)
	}
}
