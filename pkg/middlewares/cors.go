package middlewares

import "github.com/gin-gonic/gin"

// CORS sets the necessary headers to allow Cross-Origin Resource Sharing (CORS).
func CORS() func(c *gin.Context) {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Next()
	}
}
