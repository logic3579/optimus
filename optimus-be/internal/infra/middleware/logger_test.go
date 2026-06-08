package middleware_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/middleware"
)

func TestLogger_EmitsStructuredEntry(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, nil))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger(logger))
	r.GET("/things/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/things/42", nil)
	req.Header.Set("X-Request-ID", "abc")
	r.ServeHTTP(rec, req)

	out := buf.String()
	require.Contains(t, out, `"method":"GET"`)
	require.Contains(t, out, `"path":"/things/42"`)
	require.Contains(t, out, `"status":200`)
	require.Contains(t, out, `"request_id":"abc"`)
	require.Contains(t, out, `"latency_ms"`)
}

func TestLogger_LogsErrorForStatus5xx(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger(logger))
	r.GET("/boom", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	require.Contains(t, buf.String(), `"level":"ERROR"`)
}
