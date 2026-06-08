package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"optimus-be/internal/infra/config"
)

func I18n(cfg config.I18nConfig) gin.HandlerFunc {
	supported := map[string]struct{}{}
	for _, s := range cfg.Supported {
		supported[s] = struct{}{}
	}
	return func(c *gin.Context) {
		lang := cfg.DefaultLang
		raw := c.GetHeader("Accept-Language")
		if raw != "" {
			for _, part := range strings.Split(raw, ",") {
				tag := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
				if _, ok := supported[tag]; ok {
					lang = tag
					break
				}
			}
		}
		c.Set(CtxKeyLang, lang)
		c.Next()
	}
}
