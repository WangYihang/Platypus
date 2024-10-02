package middlewares

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ZapLogger logs incoming requests and their corresponding responses using a Zap logger.
func ZapLogger(logger *zap.Logger) func(c *gin.Context) {
	return func(c *gin.Context) {
		c.Set("logger", logger)

		requestID := c.GetString("request_id")
		startTime := time.Now()

		// Process the request
		c.Next()

		// Log the request and response together
		latency := time.Since(startTime)
		fields := []zap.Field{
			// Request details
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("query", c.Request.URL.RawQuery),
			zap.String("ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.String("referer", c.Request.Referer()),
			zap.String("request_id", requestID),
		}

		// Response details
		fields = append(fields,
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", latency),
			zap.Int("body_size", c.Writer.Size()),
		)

		// Error details (if any)
		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("error", c.Errors.String()))
		}

		logger.Info("Request and Response", fields...)
	}
}
