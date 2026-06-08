package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"optimus-be/internal/infra/config"
)

func CORS(cfg config.CORSConfig) gin.HandlerFunc {
	allowed := map[string]struct{}{}
	for _, o := range cfg.AllowedOrigins {
		allowed[o] = struct{}{}
	}
	methods := strings.Join(cfg.AllowedMethods, ", ")
	credentials := "false"
	if cfg.AllowCredentials {
		credentials = "true"
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if _, ok := allowed[origin]; ok && origin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Methods", methods)
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept-Language, X-Request-ID")
			c.Writer.Header().Set("Access-Control-Allow-Credentials", credentials)
			c.Writer.Header().Set("Access-Control-Max-Age", "86400")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
