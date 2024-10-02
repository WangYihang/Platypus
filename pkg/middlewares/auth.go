package middlewares

import (
	"github.com/gin-gonic/gin"
)

// AuthMiddleware is a middleware that checks if the request has the correct token.
func AuthMiddleware(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") != token {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}
