//go:build dbtest

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/config"
	"optimus-be/internal/infra/crypto"
	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/log"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/infra/ratelimit"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/auth"
	"optimus-be/internal/modules/health"
	"optimus-be/internal/modules/rbac"
	"optimus-be/internal/seed"
)

const e2eSecret = "test_secret_must_be_at_least_32_bytes_!!"

func mustJSONBody(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return bytes.NewReader(b)
}

func bodyMap(t *testing.T, r *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &out))
	return out
}

func setupServer(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	t.Cleanup(teardown)
	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{AdminUsername: "admin", AdminEmail: "a@x", BcryptCost: 4})
	require.NoError(t, err)

	hash, _ := crypto.HashPassword("S3cret-Pass!", 4)
	require.NoError(t, gdb.Model(&models.User{}).Where("username = ?", "admin").Update("password_hash", hash).Error)

	signer := crypto.NewJWTSigner(e2eSecret)
	authSvc := auth.NewService(
		auth.NewRepo(gdb), signer,
		ratelimit.NewLoginLimiter(5, time.Minute, time.Minute),
		auth.ServiceOptions{AccessTTL: time.Minute, RefreshTTL: time.Hour, BcryptCost: 4},
	)
	authHandler := auth.NewHandler(authSvc)
	cache := rbac.NewPermissionCache(gdb, time.Minute)
	meHandler := rbac.NewHandler(rbac.NewMeService(gdb, cache))

	logger := log.New(log.Options{Level: "warn", Format: "json"})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger(logger))
	r.Use(middleware.Recover(logger))
	r.Use(middleware.CORS(config.CORSConfig{AllowedOrigins: []string{"*"}, AllowedMethods: []string{"GET", "POST"}}))
	r.Use(middleware.I18n(config.I18nConfig{DefaultLang: "zh-CN", Supported: []string{"zh-CN", "en-US"}}))
	api := r.Group("/api/v1")
	(&health.Handler{DB: gdb, Version: "test"}).Register(api)
	authHandler.Register(api.Group("/auth"))
	protected := api.Group("")
	protected.Use(middleware.JWTAuth(signer))
	meHandler.RegisterMe(protected)
	return r, gdb
}

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

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		mustJSONBody(t, map[string]string{"username": "viewer1", "password": "viewer-pw-1"}))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	access := bodyMap(t, rec)["data"].(map[string]any)["access_token"].(string)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/me/menus", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "system.users")
	require.Contains(t, rec.Body.String(), "dashboard")
}
