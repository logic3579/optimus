//go:build dbtest

package credentials_test

import (
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/credentials/vault"
	"optimus-be/internal/modules/rbac"
)

func TestModule_New_BuildsAllThreeServices(t *testing.T) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer td()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	cipher, err := vault.NewCipher(key)
	require.NoError(t, err)
	m := credentials.New(gdb, cipher, audit.NewRecorder(gdb))
	require.NotNil(t, m.SSH)
	require.NotNil(t, m.Kubeconfig)
	require.NotNil(t, m.CloudKey)
	require.NotNil(t, m.Consumer)
}

func TestModule_MountRoutes_RegistersAllNineEndpoints(t *testing.T) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer td()
	_, err := permissions.Register(t.Context(), gdb, permissions.All)
	require.NoError(t, err)

	key := make([]byte, 32)
	_, _ = rand.Read(key)
	cipher, _ := vault.NewCipher(key)
	mod := credentials.New(gdb, cipher, audit.NewRecorder(gdb))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uint64(0)); c.Next() })
	cache := rbac.NewPermissionCache(gdb, time.Minute)
	protected := r.Group("/api/v1")
	mod.MountRoutes(protected, cache)

	// Hit every read endpoint with no permissions → expect 403 (gates exist).
	// With actor 0 (no real user), the cache load returns empty, so each
	// RequirePermission gate denies. We only care that the route is REGISTERED
	// (i.e. not 404) for each of the 9 endpoints.
	routes := []struct{ method, path string }{
		{"GET", "/api/v1/credentials/ssh-keys"},
		{"POST", "/api/v1/credentials/ssh-keys"},
		{"GET", "/api/v1/credentials/ssh-keys/1"},
		{"PUT", "/api/v1/credentials/ssh-keys/1"},
		{"DELETE", "/api/v1/credentials/ssh-keys/1"},
		{"GET", "/api/v1/credentials/kubeconfigs"},
		{"POST", "/api/v1/credentials/kubeconfigs"},
		{"GET", "/api/v1/credentials/cloud-keys"},
		{"POST", "/api/v1/credentials/cloud-keys"},
	}
	for _, r2 := range routes {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(r2.method, r2.path, nil)
		r.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "%s %s not registered", r2.method, r2.path)
	}
}
