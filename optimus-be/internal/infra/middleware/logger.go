package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

func Logger(l *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latencyMs := time.Since(start).Milliseconds()

		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", latencyMs,
			"request_id", c.GetString(CtxKeyRequestID),
			"remote_ip", c.ClientIP(),
		}
		if uid := c.GetUint64(CtxKeyUserID); uid != 0 {
			attrs = append(attrs, "user_id", uid)
		}
		switch {
		case c.Writer.Status() >= 500:
			l.Error("http.request", attrs...)
		default:
			l.Info("http.request", attrs...)
		}
	}
}
