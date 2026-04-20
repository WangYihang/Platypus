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
			return panicRESTfully(c, fmt.Sprintf("%s is required", param))
		}
	}
	return true
}

func paramsExistOrAbort(c *gin.Context, params []string) bool {
	for _, param := range params {
		if c.Param(param) == "" {
			return panicRESTfully(c, fmt.Sprintf("%s is required", param))
		}
	}
	return true
}

func panicRESTfully(c *gin.Context, msg string) bool {
	c.JSON(200, gin.H{
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
