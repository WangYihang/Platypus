package api

import (
	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/user"
)

// RegisterV1AuthRoutes mounts the auth + user-management endpoints
// on the supplied engine. Public routes (info / bootstrap / login)
// are unauthenticated; refresh is a deprecation shim that returns
// 410. Logout / change-password / sessions all live behind
// RequireAuth so the handler can read the caller's session id from
// the principal — no body-token handling needed.
func RegisterV1AuthRoutes(engine *gin.Engine, auth *AuthHandler, users *UsersHandler, rbac *RBAC) {
	pub := engine.Group("/api/v1/auth")
	pub.GET("/info", auth.PublicInfo)
	pub.POST("/bootstrap", auth.Bootstrap)
	pub.POST("/login", auth.Login)
	pub.POST("/refresh", auth.Refresh) // 410 Gone — kept so old clients learn the deprecation

	authed := engine.Group("/api/v1/auth")
	authed.Use(rbac.RequireAuth())
	authed.POST("/logout", auth.Logout)
	authed.PATCH("/password", auth.ChangePassword)
	authed.GET("/sessions", auth.ListSessions)
	authed.DELETE("/sessions/:sid", auth.RevokeSession)

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
