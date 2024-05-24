package middleware

import (
	"time"

	"github.com/MartinHell/overlord/logs"
	"github.com/gin-gonic/gin"
)

func LoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		// Process request
		c.Next()

		// Log the request
		endTime := time.Now()
		latency := endTime.Sub(startTime)
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method
		path := c.Request.URL.Path

		logs.Sugar.Infow("Incoming request",
			"status", statusCode,
			"method", method,
			"path", path,
			"latency", latency,
			"clientIP", clientIP,
		)
	}
}
