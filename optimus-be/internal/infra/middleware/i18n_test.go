package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/config"
	"optimus-be/internal/infra/middleware"
)

func mkI18n() gin.HandlerFunc {
	return middleware.I18n(config.I18nConfig{
		DefaultLang: "zh-CN",
		Supported:   []string{"zh-CN", "en-US"},
	})
}

func TestI18n_UsesAcceptLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mkI18n())
	var lang string
	r.GET("/", func(c *gin.Context) { lang = c.GetString(middleware.CtxKeyLang); c.Status(200) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	r.ServeHTTP(rec, req)
	require.Equal(t, "en-US", lang)
}

func TestI18n_FallsBackToDefaultForUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mkI18n())
	var lang string
	r.GET("/", func(c *gin.Context) { lang = c.GetString(middleware.CtxKeyLang); c.Status(200) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "fr-FR")
	r.ServeHTTP(rec, req)
	require.Equal(t, "zh-CN", lang)
}

func TestI18n_NoHeaderUsesDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mkI18n())
	var lang string
	r.GET("/", func(c *gin.Context) { lang = c.GetString(middleware.CtxKeyLang); c.Status(200) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, "zh-CN", lang)
}
