package controllers

import (
	"time"

	"github.com/gin-gonic/gin"
)

// HealthCheckController returns the health status of the server
func HealthCheckController(c *gin.Context) {
	c.JSON(200, gin.H{
		"healthy":   true,
		"timestamp": time.Now().UnixMilli(),
	})
}
