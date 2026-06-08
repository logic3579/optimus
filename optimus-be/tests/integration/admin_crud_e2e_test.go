//go:build dbtest

package integration_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestE2E_AdminCRUDHappyPath exercises the admin happy path across the wired
// module endpoints. It does NOT re-validate domain logic (covered by service-
// and handler-level tests); it only confirms the middleware chain + RBAC gates
// + JSON envelopes are correctly wired in main.go.
func TestE2E_AdminCRUDHappyPath(t *testing.T) {
	r, _ := setupServer(t)
	access := login(t, r, "admin", "S3cret-Pass!")
	bearer := "Bearer " + access

	do := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var req *http.Request
		if body == nil {
			req = httptest.NewRequest(method, path, nil)
		} else {
			req = httptest.NewRequest(method, path, mustJSONBody(t, body))
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", bearer)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	// --- Users -----------------------------------------------------------

	// list users (admin already exists from seed)
	rec := do(http.MethodGet, "/api/v1/users?page=1&page_size=20", nil)
	require.Equalf(t, http.StatusOK, rec.Code, "GET /users body=%s", rec.Body.String())

	// create a new user
	rec = do(http.MethodPost, "/api/v1/users", map[string]any{
		"username":     "alice",
		"email":        "alice@example.com",
		"password":     "Alice-Pass-1!",
		"display_name": "Alice",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "POST /users body=%s", rec.Body.String())
	created := bodyMap(t, rec)["data"].(map[string]any)
	aliceID := uint64(created["id"].(float64))
	require.NotZero(t, aliceID)

	// fetch the created user
	rec = do(http.MethodGet, "/api/v1/users/"+itoa(aliceID), nil)
	require.Equalf(t, http.StatusOK, rec.Code, "GET /users/:id body=%s", rec.Body.String())

	// update display name
	rec = do(http.MethodPut, "/api/v1/users/"+itoa(aliceID), map[string]any{
		"display_name": "Alice Liddell",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "PUT /users/:id body=%s", rec.Body.String())
	require.Contains(t, rec.Body.String(), "Alice Liddell")

	// reset password (different perm gate)
	rec = do(http.MethodPut, "/api/v1/users/"+itoa(aliceID)+"/password", map[string]any{
		"password": "New-Alice-Pass-2!",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "PUT /users/:id/password body=%s", rec.Body.String())

	// disable user (set status)
	rec = do(http.MethodPut, "/api/v1/users/"+itoa(aliceID)+"/status", map[string]any{
		"status": "disabled",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "PUT /users/:id/status body=%s", rec.Body.String())

	// delete user
	rec = do(http.MethodDelete, "/api/v1/users/"+itoa(aliceID), nil)
	require.Equalf(t, http.StatusOK, rec.Code, "DELETE /users/:id body=%s", rec.Body.String())

	// --- Roles -----------------------------------------------------------

	rec = do(http.MethodGet, "/api/v1/roles", nil)
	require.Equalf(t, http.StatusOK, rec.Code, "GET /roles body=%s", rec.Body.String())
	require.Contains(t, rec.Body.String(), `"code":"admin"`)
	require.Contains(t, rec.Body.String(), `"code":"viewer"`)

	rec = do(http.MethodPost, "/api/v1/roles", map[string]any{
		"code": "editor",
		"name": "Editor",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "POST /roles body=%s", rec.Body.String())
	editor := bodyMap(t, rec)["data"].(map[string]any)
	editorID := uint64(editor["id"].(float64))

	rec = do(http.MethodPut, "/api/v1/roles/"+itoa(editorID)+"/permissions", map[string]any{
		"permission_codes": []string{"system:user:read", "system:role:read"},
	})
	require.Equalf(t, http.StatusOK, rec.Code, "PUT /roles/:id/permissions body=%s", rec.Body.String())
	require.Contains(t, rec.Body.String(), "system:user:read")

	rec = do(http.MethodDelete, "/api/v1/roles/"+itoa(editorID), nil)
	require.Equalf(t, http.StatusOK, rec.Code, "DELETE /roles/:id body=%s", rec.Body.String())

	// --- Permissions (read-only) ----------------------------------------

	rec = do(http.MethodGet, "/api/v1/permissions", nil)
	require.Equalf(t, http.StatusOK, rec.Code, "GET /permissions body=%s", rec.Body.String())
	require.Contains(t, rec.Body.String(), "system:user:read")

	// --- Menus -----------------------------------------------------------

	rec = do(http.MethodGet, "/api/v1/menus", nil)
	require.Equalf(t, http.StatusOK, rec.Code, "GET /menus body=%s", rec.Body.String())
	require.Contains(t, rec.Body.String(), "system.users")

	rec = do(http.MethodPost, "/api/v1/menus", map[string]any{
		"code": "reports",
		"name": "menu.reports",
		"path": "/reports",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "POST /menus body=%s", rec.Body.String())
	mn := bodyMap(t, rec)["data"].(map[string]any)
	menuID := uint64(mn["id"].(float64))

	rec = do(http.MethodDelete, "/api/v1/menus/"+itoa(menuID), nil)
	require.Equalf(t, http.StatusOK, rec.Code, "DELETE /menus/:id body=%s", rec.Body.String())

	// --- Audit -----------------------------------------------------------

	rec = do(http.MethodGet, "/api/v1/audit-logs?page=1&page_size=20", nil)
	require.Equalf(t, http.StatusOK, rec.Code, "GET /audit-logs body=%s", rec.Body.String())
	// The create/update/delete user calls above should have produced rows.
	require.Contains(t, rec.Body.String(), `"items"`)
}

// itoa is a tiny wrapper kept so the test code reads like "GET /users/" + itoa(id).
func itoa(n uint64) string { return strconv.FormatUint(n, 10) }
