package middleware

import (
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

func Recover(l *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				l.Error("panic recovered",
					"panic", fmt.Sprintf("%v", rec),
					"stack", string(debug.Stack()),
					"request_id", c.GetString(CtxKeyRequestID),
					"path", c.Request.URL.Path,
				)
				if !c.Writer.Written() {
					response.Error(c, apperr.New(apperr.CodeInternal, "common.internal", "internal server error"))
				}
				c.Abort()
			}
		}()
		c.Next()
	}
}
