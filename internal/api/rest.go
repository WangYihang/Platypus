package api

import (
	"fmt"
	"io"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func paramsExistOrAbort(c *gin.Context, params []string) bool {
	for _, param := range params {
		if c.Param(param) == "" {
			return abortWithError(c, 400, fmt.Sprintf("%s is required", param))
		}
	}
	return true
}

// abortWithError emits {error: msg} at the given status and aborts the gin
// handler chain. Used by middleware/utility paths that need to bail before
// the normal handler body runs (e.g. websocket upgrade path parameter checks).
func abortWithError(c *gin.Context, status int, msg string) bool {
	c.JSON(status, gin.H{"error": msg})
	c.Abort()
	return false
}

// CreateRESTfulAPIServer assembles the bare gin engine with CORS middleware.
// Routes are registered separately via RegisterWebSocketRoutes and
// RegisterV1Routes — both wired up by the caller.
func CreateRESTfulAPIServer() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = io.Discard
	engine := gin.Default()

	engine.Use(cors.New(cors.Config{
		AllowOrigins:  []string{"*"},
		AllowMethods:  []string{"GET", "POST", "DELETE", "PUT", "PATCH"},
		AllowHeaders:  []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders: []string{"Content-Length"},
		MaxAge:        12 * time.Hour,
	}))

	return engine
}
