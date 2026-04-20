package api

import (
	ginSwagger "github.com/swaggo/gin-swagger"
	swaggerFiles "github.com/swaggo/files"

	"github.com/gin-gonic/gin"
)

// RegisterSwaggerRoutes mounts the interactive swagger UI at /swagger/*any.
// Call after CreateRESTfulAPIServer. Placed outside any auth group so the
// docs themselves are browsable without a token — the endpoints they describe
// still require Bearer auth, which is what matters for security.
func RegisterSwaggerRoutes(engine *gin.Engine) {
	engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
