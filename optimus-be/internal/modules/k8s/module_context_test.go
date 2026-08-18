package k8s

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/middleware"
)

func TestWithCredentialActorBridgesAuthenticatedRequestContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.CtxKeyUserID, uint64(42))
		c.Next()
	})
	r.Use(withCredentialActor())

	var original context.Context
	var bridged context.Context
	r.GET("/", func(c *gin.Context) {
		bridged = c.Request.Context()
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	original = req.Context()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.NotEqual(t, original, bridged)
}

func TestWithCredentialActorLeavesAnonymousRequestContextUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(withCredentialActor())

	var got context.Context
	r.GET("/", func(c *gin.Context) {
		got = c.Request.Context()
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	original := req.Context()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, original, got)
}
