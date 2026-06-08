package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
)

func TestRecover_ConvertsPanicToInternalEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, nil))

	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Recover(logger))
	r.GET("/boom", func(c *gin.Context) { panic("oops") })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, float64(apperr.CodeInternal), body["code"])
	require.Equal(t, "internal server error", body["message"])

	require.Contains(t, buf.String(), `"panic":"oops"`)
	require.NotContains(t, rec.Body.String(), "oops")
}
