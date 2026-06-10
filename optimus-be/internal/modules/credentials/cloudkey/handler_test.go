//go:build dbtest

package cloudkey_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials/cloudkey"
	"optimus-be/internal/seed"
)

func newHandlerRouter(t *testing.T) *gin.Engine {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(td)
	ctx := context.Background()
	_, err := permissions.Register(ctx, gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(ctx, gdb, seed.Options{AdminUsername: "admin", AdminEmail: "a@x", BcryptCost: 4})
	require.NoError(t, err)
	h := cloudkey.NewHandler(cloudkey.NewService(cloudkey.NewRepo(gdb), passthroughCipher{}, audit.NewRecorder(gdb)))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uint64(1)); c.Next() })
	g := r.Group("/api/v1/credentials/cloud-keys")
	g.GET("", h.HandleList())
	g.POST("", h.HandleCreate())
	g.GET("/:id", h.HandleGet())
	g.PUT("/:id", h.HandleUpdate())
	g.DELETE("/:id", h.HandleDelete())
	return r
}

func doJSON(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestHandler_CreateAndNoSecretLeak(t *testing.T) {
	r := newHandlerRouter(t)
	w := doJSON(t, r, "POST", "/api/v1/credentials/cloud-keys", cloudkey.CreateRequest{
		Name: "h-aws", Provider: "aws", Region: "us-east-1",
		AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "verysecret123",
	})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.False(t, bytes.Contains(w.Body.Bytes(), []byte("verysecret123")),
		"response leaks plaintext secret_access_key")
	require.False(t, bytes.Contains(w.Body.Bytes(), []byte("AKIAEXAMPLE")),
		"response leaks plaintext access_key_id")

	var env struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	id := uint64(env.Data["id"].(float64))

	// List shouldn't leak either.
	w = doJSON(t, r, "GET", "/api/v1/credentials/cloud-keys?page=1&page_size=10", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.False(t, bytes.Contains(w.Body.Bytes(), []byte("verysecret123")))
	require.False(t, bytes.Contains(w.Body.Bytes(), []byte("AKIAEXAMPLE")))

	// Get
	w = doJSON(t, r, "GET", "/api/v1/credentials/cloud-keys/"+strconv.FormatUint(id, 10), nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"provider":"aws"`)

	// Delete
	w = doJSON(t, r, "DELETE", "/api/v1/credentials/cloud-keys/"+strconv.FormatUint(id, 10), nil)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_RejectsBadProvider(t *testing.T) {
	r := newHandlerRouter(t)
	w := doJSON(t, r, "POST", "/api/v1/credentials/cloud-keys", map[string]any{
		"name":              "ibm-key",
		"provider":          "ibm",
		"access_key_id":     "key",
		"secret_access_key": "sec",
	})
	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	require.Contains(t, w.Body.String(), "common.validation")
}

func TestHandler_Update_RotateSecret(t *testing.T) {
	r := newHandlerRouter(t)
	w := doJSON(t, r, "POST", "/api/v1/credentials/cloud-keys", cloudkey.CreateRequest{
		Name: "h-u", Provider: "aws", Region: "us-east-1",
		AccessKeyID: "AK1", SecretAccessKey: "S1",
	})
	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	id := uint64(env.Data["id"].(float64))

	newSecret := "S2"
	w = doJSON(t, r, "PUT", "/api/v1/credentials/cloud-keys/"+strconv.FormatUint(id, 10),
		cloudkey.UpdateRequest{SecretAccessKey: &newSecret})
	require.Equal(t, http.StatusOK, w.Code)
	require.False(t, bytes.Contains(w.Body.Bytes(), []byte("S2")),
		"PUT response leaks the new secret")
}
