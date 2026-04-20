package api

import (
	"fmt"
	"io"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func formExistOrAbort(c *gin.Context, params []string) bool {
	for _, param := range params {
		if c.PostForm(param) == "" {
			return abortWithLegacyError(c, 400, fmt.Sprintf("%s is required", param))
		}
	}
	return true
}

func paramsExistOrAbort(c *gin.Context, params []string) bool {
	for _, param := range params {
		if c.Param(param) == "" {
			return abortWithLegacyError(c, 400, fmt.Sprintf("%s is required", param))
		}
	}
	return true
}

// abortWithLegacyError emits the {status:false, msg} envelope at the given
// HTTP status code and aborts the gin handler chain. The legacy envelope is
// kept for client compatibility; only the status code is corrected so that
// generic HTTP tooling (curl -f, retry middleware, monitors) sees the error.
func abortWithLegacyError(c *gin.Context, status int, msg string) bool {
	c.JSON(status, gin.H{
		"status": false,
		"msg":    msg,
	})
	c.Abort()
	return false
}

// CreateRESTfulAPIServer assembles the bare gin engine with CORS middleware.
// Routes are registered separately via RegisterWebSocketRoutes,
// RegisterLegacyRoutes, and RegisterV1Routes — all wired up by the caller.
func CreateRESTfulAPIServer() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = io.Discard
	engine := gin.Default()

	engine.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "DELETE", "PUT", "PATCH"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	return engine
}
