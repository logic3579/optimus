//go:build dbtest

package httpcredential_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestHandlerRejectsBearerUsernameEvenWhenEmpty(t *testing.T) {
	svc, _, td := setup(t)
	defer td()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := httpcredential.NewHandler(svc)
	r.POST("/credentials/http-credentials", h.HandleCreate())
	for _, body := range []string{`{"name":"b1","auth_type":"bearer","username":"","secret":"x"}`, `{"name":"b2","auth_type":"bearer","username":"supplied","secret":"x"}`} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/credentials/http-credentials", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
	}
}

func TestHandlerListGetUpdateDeleteAndValidation(t *testing.T) {
	svc, _, td := setup(t)
	defer td()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := httpcredential.NewHandler(svc)
	g := r.Group("/credentials/http-credentials")
	g.GET("", h.HandleList())
	g.GET("/:id", h.HandleGet())
	g.POST("", h.HandleCreate())
	g.PUT("/:id", h.HandleUpdate())
	g.DELETE("/:id", h.HandleDelete())
	d, err := svc.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{Name: "crud", AuthType: "bearer", Secret: "hidden"})
	require.NoError(t, err)
	cases := []struct {
		method, path, body string
		status             int
	}{{http.MethodGet, "/credentials/http-credentials", "", 200}, {http.MethodGet, "/credentials/http-credentials/" + strconv.FormatUint(d.ID, 10), "", 200}, {http.MethodPut, "/credentials/http-credentials/" + strconv.FormatUint(d.ID, 10), `{"name":"updated"}`, 200}, {http.MethodGet, "/credentials/http-credentials/bad", "", 400}, {http.MethodPost, "/credentials/http-credentials", `{"name":"basic-missing","auth_type":"basic","secret":"x"}`, 400}, {http.MethodDelete, "/credentials/http-credentials/" + strconv.FormatUint(d.ID, 10), "", 200}, {http.MethodGet, "/credentials/http-credentials/" + strconv.FormatUint(d.ID, 10), "", 404}}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		require.Equal(t, tc.status, w.Code, "%s %s: %s", tc.method, tc.path, w.Body.String())
		require.NotContains(t, w.Body.String(), "hidden")
		require.NotContains(t, w.Body.String(), "ciphertext")
	}
}
