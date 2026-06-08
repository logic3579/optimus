//go:build dbtest

package integration_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/crypto"
	"optimus-be/internal/models"
)

// TestE2E_ViewerCanReadCannotWrite asserts the per-route RBAC gates wired in
// main.go actually enforce permissions: the seeded "viewer" role gets only
// "*:read" permissions, so viewer1 must succeed on GET /users but be denied
// on POST /users.
func TestE2E_ViewerCanReadCannotWrite(t *testing.T) {
	r, gdb := setupServer(t)

	// Create viewer1 with the seeded "viewer" role (read-only perms).
	hash, _ := crypto.HashPassword("viewer-pw-1", 4)
	v := &models.User{Username: "viewer1", Email: "v@x", PasswordHash: hash, Status: "enabled"}
	require.NoError(t, gdb.Create(v).Error)
	var viewer models.Role
	require.NoError(t, gdb.Where("code = ?", "viewer").First(&viewer).Error)
	require.NoError(t, gdb.Create(&models.UserRole{UserID: v.ID, RoleID: viewer.ID}).Error)

	access := login(t, r, "viewer1", "viewer-pw-1")
	bearer := "Bearer " + access

	// GET /users — viewer has system:user:read → 200
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users?page=1&page_size=20", nil)
	req.Header.Set("Authorization", bearer)
	r.ServeHTTP(rec, req)
	require.Equalf(t, http.StatusOK, rec.Code, "viewer GET /users body=%s", rec.Body.String())

	// POST /users — viewer lacks system:user:write → 403
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users", mustJSONBody(t, map[string]any{
		"username": "mallory",
		"email":    "mallory@example.com",
		"password": "Mallory-1-Pass!",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearer)
	r.ServeHTTP(rec, req)
	require.Equalf(t, http.StatusForbidden, rec.Code, "viewer POST /users body=%s", rec.Body.String())

	// DELETE /users/:id — viewer lacks system:user:delete → 403
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/users/9999", nil)
	req.Header.Set("Authorization", bearer)
	r.ServeHTTP(rec, req)
	require.Equalf(t, http.StatusForbidden, rec.Code, "viewer DELETE /users/:id body=%s", rec.Body.String())

	// POST /menus — viewer lacks system:menu:write → 403 (different module, same story)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/menus", mustJSONBody(t, map[string]any{
		"code": "x", "name": "x", "path": "/x",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearer)
	r.ServeHTTP(rec, req)
	require.Equalf(t, http.StatusForbidden, rec.Code, "viewer POST /menus body=%s", rec.Body.String())
}

// TestE2E_UnauthenticatedRejected confirms that hitting any protected route
// without a Bearer token yields 401 (and never reaches the RBAC gate).
func TestE2E_UnauthenticatedRejected(t *testing.T) {
	r, _ := setupServer(t)

	for _, path := range []string{
		"/api/v1/users",
		"/api/v1/roles",
		"/api/v1/permissions",
		"/api/v1/menus",
		"/api/v1/audit-logs",
		"/api/v1/me",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(rec, req)
		require.Equalf(t, http.StatusUnauthorized, rec.Code, "GET %s should be 401, body=%s", path, rec.Body.String())
	}
}
