package httpcredential

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHandlerCRUDRedactsAuthAndRejectsInvalidInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(&behaviorRepo{}, copyCipher{}, nil)
	handler := NewHandler(svc)
	router := gin.New()
	handler.Mount(router.Group("/credentials/http-credentials"), func(string) gin.HandlerFunc {
		return func(c *gin.Context) { c.Next() }
	})

	const auth = "handler-secret-value"
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodPost, "/credentials/http-credentials", `{"name":"prom","auth_type":"bearer","secret":"` + auth + `"}`, http.StatusOK},
		{http.MethodGet, "/credentials/http-credentials", "", http.StatusOK},
		{http.MethodGet, "/credentials/http-credentials/1", "", http.StatusOK},
		{http.MethodPut, "/credentials/http-credentials/1", `{"name":"renamed"}`, http.StatusOK},
		{http.MethodGet, "/credentials/http-credentials/0", "", http.StatusBadRequest},
		{http.MethodGet, "/credentials/http-credentials/bad", "", http.StatusBadRequest},
		{http.MethodPost, "/credentials/http-credentials", "{", http.StatusBadRequest},
		{http.MethodPut, "/credentials/http-credentials/1", "{", http.StatusBadRequest},
		{http.MethodDelete, "/credentials/http-credentials/1", "", http.StatusOK},
		{http.MethodGet, "/credentials/http-credentials/1", "", http.StatusNotFound},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		require.Equal(t, tc.status, w.Code, "%s %s: %s", tc.method, tc.path, w.Body.String())
		require.NotContains(t, w.Body.String(), auth)
		require.NotContains(t, w.Body.String(), "secret_ciphertext")
	}
}

func TestHandlerServiceErrorsUseSafeEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const internalDetail = "database unavailable"
	repo := &behaviorRepo{getErr: errors.New(internalDetail), listErr: errors.New(internalDetail)}
	handler := NewHandler(NewService(repo, copyCipher{}, nil))
	router := gin.New()
	router.GET("/list", handler.HandleList())
	router.GET("/get/:id", handler.HandleGet())
	router.DELETE("/delete/:id", handler.HandleDelete())
	for _, path := range []string{"/list", "/get/1", "/delete/1"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if strings.HasPrefix(path, "/delete") {
			w = httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, path, nil))
		}
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.NotContains(t, w.Body.String(), internalDetail)
	}
}

func TestCipherAuthFailureIsAbsentFromResponseAndLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const auth = "raw-auth-must-not-leak"
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	handler := NewHandler(NewService(&behaviorRepo{}, faultCipher{sealErr: errors.New(auth)}, nil))
	router := gin.New()
	router.POST("/credentials", handler.HandleCreate())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/credentials", strings.NewReader(`{"name":"prom","auth_type":"bearer","secret":"`+auth+`"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.NotContains(t, w.Body.String(), auth)
	require.NotContains(t, logs.String(), auth)
}

func TestSwaggerCredentialResponsesExcludeSecretFields(t *testing.T) {
	raw, err := os.ReadFile("../../../../api/docs/swagger.json")
	require.NoError(t, err)
	var document struct {
		Definitions map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"definitions"`
	}
	require.NoError(t, json.Unmarshal(raw, &document))
	checked := 0
	for name, schema := range document.Definitions {
		if !strings.HasPrefix(name, "internal_modules_credentials_httpcredential.") || strings.HasSuffix(name, "Request") {
			continue
		}
		checked++
		require.NotContains(t, schema.Properties, "secret", name)
		require.NotContains(t, schema.Properties, "secret_ciphertext", name)
	}
	require.Positive(t, checked, "expected generated HTTP credential response schemas")
}
