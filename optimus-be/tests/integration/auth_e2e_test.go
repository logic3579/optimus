//go:build dbtest

package integration_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/crypto"
	"optimus-be/internal/models"
)

func TestE2E_LoginRefreshReplayLogout(t *testing.T) {
	r, _ := setupServer(t)

	// 1) Login
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		mustJSONBody(t, map[string]string{"username": "admin", "password": "S3cret-Pass!"}))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	body := bodyMap(t, rec)
	data := body["data"].(map[string]any)
	access1 := data["access_token"].(string)
	refresh1 := data["refresh_token"].(string)
	require.NotEmpty(t, access1)
	require.NotEmpty(t, refresh1)

	// 2) Hit /me with access1
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+access1)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"username":"admin"`)

	// 3) Refresh
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		mustJSONBody(t, map[string]string{"refresh_token": refresh1}))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	data2 := bodyMap(t, rec)["data"].(map[string]any)
	access2 := data2["access_token"].(string)
	refresh2 := data2["refresh_token"].(string)
	require.NotEqual(t, access1, access2)
	require.NotEqual(t, refresh1, refresh2)

	// 4) Replay refresh1 — must 401 with refresh_replay code
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		mustJSONBody(t, map[string]string{"refresh_token": refresh1}))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.True(t, strings.Contains(rec.Body.String(), "refresh_replay") || strings.Contains(rec.Body.String(), "40104"))

	// 5) After replay, refresh2 should also be revoked
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		mustJSONBody(t, map[string]string{"refresh_token": refresh2}))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.NotEqual(t, http.StatusOK, rec.Code)
}

func TestE2E_MeMenusFiltersByPermission(t *testing.T) {
	r, gdb := setupServer(t)

	hash, _ := crypto.HashPassword("viewer-pw-1", 4)
	v := &models.User{Username: "viewer1", Email: "v@x", PasswordHash: hash, Status: "enabled"}
	require.NoError(t, gdb.Create(v).Error)
	var viewer models.Role
	require.NoError(t, gdb.Where("code = ?", "viewer").First(&viewer).Error)
	require.NoError(t, gdb.Create(&models.UserRole{UserID: v.ID, RoleID: viewer.ID}).Error)

	access := login(t, r, "viewer1", "viewer-pw-1")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/menus", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "system.users")
	require.Contains(t, rec.Body.String(), "dashboard")
}
