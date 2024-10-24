package middlewares

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware is a middleware that checks if the request has the correct Bearer token.
func AuthMiddleware(expectedToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get the Authorization header
		authorizationHeader := c.GetHeader("Authorization")
		if authorizationHeader == "" {
			c.AbortWithStatusJSON(401, gin.H{"error": "missing authorization header"})
			return
		}

		// Check if the header starts with "Bearer " and extract the token
		if !strings.HasPrefix(authorizationHeader, "Bearer ") {
			c.AbortWithStatusJSON(401, gin.H{"error": "invalid authorization scheme"})
			return
		}

		// Extract the token
		token := strings.TrimPrefix(authorizationHeader, "Bearer ")

		// Check if the token matches the expected token
		if token != expectedToken {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
			return
		}

		// Token is valid; proceed to the next handler
		c.Next()
	}
}
