//go:build dbtest

package user_test

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
	"optimus-be/internal/modules/user"
	"optimus-be/internal/seed"
)

func newHandlerRouter(t *testing.T) (*gin.Engine, uint64) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	t.Cleanup(td)
	ctx := context.Background()
	_, err := permissions.Register(ctx, gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(ctx, gdb, seed.Options{AdminUsername: "admin", AdminEmail: "a@x", BcryptCost: 4})
	require.NoError(t, err)

	cache := rbac.NewPermissionCache(gdb, time.Minute)
	rec := audit.NewRecorder(gdb)
	svc := user.NewService(user.NewRepo(gdb), cache, rec, user.ServiceOptions{BcryptCost: 4, AdminUsername: "admin"})
	h := user.NewHandler(svc)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Stub: pretend JWT auth set actor=1 (admin)
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uint64(1)); c.Next() })
	api := r.Group("/api/v1")
	h.Register(api.Group("/users"))
	return r, 1
}

func TestHandler_CreateAndList(t *testing.T) {
	r, _ := newHandlerRouter(t)

	body, _ := json.Marshal(user.CreateRequest{Username: "alice", Email: "alice@example.com", Password: "Pass1234"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/users?page_size=5", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "alice")
	require.Contains(t, rec.Body.String(), `"total":2`)
}

func TestHandler_DeleteSelfRejected(t *testing.T) {
	r, actor := newHandlerRouter(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/"+strconv.FormatUint(actor, 10), nil)
	r.ServeHTTP(rec, req)
	// CodeCannotDeleteSelf (40906) is in the 409xx range, mapped to HTTP 409.
	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":40906`)
}

// createUser POSTs a user via the handler and returns its id. Test helper so the
// CRUD-fanout tests below stay readable.
func createUser(t *testing.T, r *gin.Engine, req user.CreateRequest) uint64 {
	t.Helper()
	body, _ := json.Marshal(req)
	rec := httptest.NewRecorder()
	httpReq := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, httpReq)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return uint64(resp["data"].(map[string]any)["id"].(float64))
}

func TestHandler_GetAndUpdate(t *testing.T) {
	r, _ := newHandlerRouter(t)
	id := createUser(t, r, user.CreateRequest{Username: "alice", Email: "alice@example.com", Password: "Pass1234"})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users/"+strconv.FormatUint(id, 10), nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"username":"alice"`)

	newName := "Alice Cooper"
	body, _ := json.Marshal(user.UpdateRequest{DisplayName: &newName})
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/"+strconv.FormatUint(id, 10), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"display_name":"Alice Cooper"`)
}

func TestHandler_DeleteOK(t *testing.T) {
	r, _ := newHandlerRouter(t)
	id := createUser(t, r, user.CreateRequest{Username: "tobegone", Email: "g@x.io", Password: "Pass1234"})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/users/"+strconv.FormatUint(id, 10), nil))
	require.Equal(t, http.StatusOK, rec.Code)

	// 404 after delete.
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users/"+strconv.FormatUint(id, 10), nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_SetRolesAndStatusAndPassword(t *testing.T) {
	r, _ := newHandlerRouter(t)
	id := createUser(t, r, user.CreateRequest{Username: "alice", Email: "alice@example.com", Password: "Pass1234"})

	// SetRoles
	body, _ := json.Marshal(user.SetRolesRequest{RoleIDs: []uint64{}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/"+strconv.FormatUint(id, 10)+"/roles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// SetStatus
	body, _ = json.Marshal(user.SetStatusRequest{Status: "disabled"})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/users/"+strconv.FormatUint(id, 10)+"/status", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"disabled"`)

	// SetPassword (admin reset)
	body, _ = json.Marshal(user.SetPasswordRequest{Password: "newpass5678"})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/users/"+strconv.FormatUint(id, 10)+"/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_BadID(t *testing.T) {
	r, _ := newHandlerRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users/abc", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
