//go:build dbtest

package role_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/rbac"
	"optimus-be/internal/modules/role"
	"optimus-be/internal/seed"
)

func newHandlerRouter(t *testing.T) *gin.Engine {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	t.Cleanup(td)
	ctx := context.Background()
	_, err := permissions.Register(ctx, gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(ctx, gdb, seed.Options{AdminUsername: "admin", AdminEmail: "a@x", BcryptCost: 4})
	require.NoError(t, err)
	cache := rbac.NewPermissionCache(gdb, time.Minute)
	rec := audit.NewRecorder(gdb)
	h := role.NewHandler(role.NewService(role.NewRepo(gdb), cache, rec))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uint64(1)); c.Next() })
	api := r.Group("/api/v1")
	h.Register(api.Group("/roles"))
	return r
}

func TestHandler_CreateAndSetPermissions(t *testing.T) {
	r := newHandlerRouter(t)

	body, _ := json.Marshal(role.CreateRequest{Code: "ops", Name: "Ops"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/roles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	id := uint64(resp["data"].(map[string]any)["id"].(float64))

	body, _ = json.Marshal(role.SetPermissionsRequest{PermissionCodes: []string{"system:user:read"}})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/roles/"+strconv.FormatUint(id, 10)+"/permissions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "system:user:read")
}

func TestHandler_BuiltinDeleteRejected(t *testing.T) {
	r := newHandlerRouter(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "admin")
	// admin role id=1 in seeded order (admin inserted first per seed.ensureBuiltinRoles)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/roles/1", nil)
	r.ServeHTTP(rec, req)
	// CodeBuiltinRoleImmutable (40905) is in the 409xx range, mapped to HTTP 409.
	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":40905`)
}
