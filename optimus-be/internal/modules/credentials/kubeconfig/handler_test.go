//go:build dbtest

package kubeconfig_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials/kubeconfig"
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
	rec := audit.NewRecorder(gdb)
	h := kubeconfig.NewHandler(kubeconfig.NewService(kubeconfig.NewRepo(gdb), passthroughCipher{}, rec))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uint64(1)); c.Next() })
	g := r.Group("/api/v1/credentials/kubeconfigs")
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

func TestHandler_CreateAndConfirmNoYAMLLeak(t *testing.T) {
	r := newHandlerRouter(t)
	w := doJSON(t, r, "POST", "/api/v1/credentials/kubeconfigs", kubeconfig.CreateRequest{
		Name: "h-kc", DefaultNamespace: "default", Kubeconfig: validKubeconfigYAML,
	})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.False(t, bytes.Contains(w.Body.Bytes(), []byte(strings.TrimSpace(validKubeconfigYAML))),
		"response leaks plaintext kubeconfig")

	var env struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Equal(t, 0, env.Code)
	id := uint64(env.Data["id"].(float64))

	// Get
	w = doJSON(t, r, "GET", "/api/v1/credentials/kubeconfigs/"+strconv.FormatUint(id, 10), nil)
	require.Equal(t, http.StatusOK, w.Code)

	// Delete
	w = doJSON(t, r, "DELETE", "/api/v1/credentials/kubeconfigs/"+strconv.FormatUint(id, 10), nil)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_ListAndUpdate(t *testing.T) {
	r := newHandlerRouter(t)
	w := doJSON(t, r, "POST", "/api/v1/credentials/kubeconfigs", kubeconfig.CreateRequest{
		Name: "lu-kc", DefaultNamespace: "default", Kubeconfig: validKubeconfigYAML,
	})
	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	id := uint64(env.Data["id"].(float64))

	// list
	w = doJSON(t, r, "GET", "/api/v1/credentials/kubeconfigs?page=1&page_size=5", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "lu-kc")

	// update default_namespace
	newNS := "kube-system"
	w = doJSON(t, r, "PUT", "/api/v1/credentials/kubeconfigs/"+strconv.FormatUint(id, 10),
		kubeconfig.UpdateRequest{DefaultNamespace: &newNS})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.Contains(t, w.Body.String(), `"default_namespace":"kube-system"`)
}

func TestHandler_RejectsBadYAML(t *testing.T) {
	r := newHandlerRouter(t)
	w := doJSON(t, r, "POST", "/api/v1/credentials/kubeconfigs", kubeconfig.CreateRequest{
		Name: "bad", Kubeconfig: "{not yaml",
	})
	require.NotEqual(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "credentials.invalid_key_format")
}
