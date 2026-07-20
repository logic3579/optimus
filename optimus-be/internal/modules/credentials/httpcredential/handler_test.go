//go:build dbtest

package httpcredential_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/modules/credentials/httpcredential"
)

func TestHandlerCreateUsesEnvelopeAndRedactsSecret(t *testing.T) {
	svc, _, td := setup(t)
	defer td()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uint64(0)); c.Next() })
	h := httpcredential.NewHandler(svc)
	r.POST("/credentials/http-credentials", h.HandleCreate())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/credentials/http-credentials", bytes.NewBufferString(`{"name":"prom","auth_type":"bearer","secret":"do-not-leak"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"code":0`)
	require.NotContains(t, w.Body.String(), "do-not-leak")
	require.NotContains(t, w.Body.String(), "ciphertext")
}
