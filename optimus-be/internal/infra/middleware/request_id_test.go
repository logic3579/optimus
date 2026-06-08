package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/middleware"
)

func TestRequestID_GeneratesWhenAbsent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	var captured string
	r.GET("/", func(c *gin.Context) {
		captured = c.GetString(middleware.CtxKeyRequestID)
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(rec, req)

	require.Len(t, captured, 32)
	require.Equal(t, captured, rec.Header().Get("X-Request-ID"))
}

func TestRequestID_PreservesInbound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	var captured string
	r.GET("/", func(c *gin.Context) {
		captured = c.GetString(middleware.CtxKeyRequestID)
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "client-supplied-id")
	r.ServeHTTP(rec, req)

	require.Equal(t, "client-supplied-id", captured)
	require.Equal(t, "client-supplied-id", rec.Header().Get("X-Request-ID"))
}
