//go:build dbtest

// Package integration_test exercises the wired HTTP server end-to-end.
//
// helpers.go contains the shared setup used by every e2e test:
//   - setupServer: start a real Postgres via dockertest, run migrations, seed
//     builtin roles/permissions/menus and an "admin" user with a known password.
//     The returned engine has the full middleware chain plus every module
//     mounted under per-route RequirePermission gates that mirror cmd/server.
//   - login: small helper that POSTs /auth/login and returns the access token.
//   - mountXxx helpers: extracted from main.go so the e2e tests stay in sync
//     with the production routing table.
package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/auth"
	"optimus-be/internal/modules/health"
	"optimus-be/internal/modules/menu"
	"optimus-be/internal/modules/permission"
	"optimus-be/internal/modules/rbac"
	"optimus-be/internal/modules/role"
	"optimus-be/internal/modules/user"
	"optimus-be/internal/seed"
)

// e2eSecret is a 40-byte string used as the JWT HMAC key in tests; long enough
// to satisfy config.Load's >=32-byte validation if the helpers were ever reused
// from a config-loading test.
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

// setupServer boots a fresh Postgres (dockertest), runs migrations, seeds the
// builtin RBAC graph, and returns a gin engine wired up exactly like cmd/server
// does (auth + /me + user + role + permission + menu + audit) so e2e tests
// touch the same middleware chain the real server uses.
//
// The seeded admin password is replaced with the deterministic value
// "S3cret-Pass!" so tests can log in without parsing the random initial pw.
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
		ratelimit.NewLoginLimiter(50, time.Minute, time.Minute),
		auth.ServiceOptions{AccessTTL: time.Minute, RefreshTTL: time.Hour, BcryptCost: 4},
	)
	authHandler := auth.NewHandler(authSvc)
	cache := rbac.NewPermissionCache(gdb, time.Minute)
	auditRec := audit.NewRecorder(gdb)
	userSvc := user.NewService(user.NewRepo(gdb), cache, auditRec, user.ServiceOptions{BcryptCost: 4, AdminUsername: "admin"})
	meHandler := rbac.NewHandler(rbac.NewMeService(gdb, cache, userSvc))

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

	mountUser(protected, user.NewHandler(userSvc), cache)

	roleSvc := role.NewService(role.NewRepo(gdb), cache, auditRec)
	mountRole(protected, role.NewHandler(roleSvc), cache)

	mountPerm(protected, permission.NewHandler(gdb), cache)

	menuSvc := menu.NewService(menu.NewRepo(gdb), auditRec)
	mountMenu(protected, menu.NewHandler(menuSvc), cache)

	auditSvc := audit.NewService(audit.NewRepo(gdb))
	mountAudit(protected, audit.NewHandler(auditSvc), cache)

	return r, gdb
}

// login POSTs /auth/login and returns the access token; fails the test on any
// non-OK response.
func login(t *testing.T, r *gin.Engine, username, password string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		mustJSONBody(t, map[string]string{"username": username, "password": password}))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equalf(t, http.StatusOK, rec.Code, "login %s failed: %s", username, rec.Body.String())
	return bodyMap(t, rec)["data"].(map[string]any)["access_token"].(string)
}

// The mount* helpers below mirror cmd/server/main.go. Keep them in sync.

func mountUser(protected *gin.RouterGroup, h *user.Handler, cache *rbac.PermissionCache) {
	g := protected.Group("/users")
	rd := g.Group("", middleware.RequirePermission(cache, "system:user:read"))
	rd.GET("", h.HandleList())
	rd.GET("/:id", h.HandleGet())
	wr := g.Group("", middleware.RequirePermission(cache, "system:user:write"))
	wr.POST("", h.HandleCreate())
	wr.PUT("/:id", h.HandleUpdate())
	wr.PUT("/:id/roles", h.HandleSetRoles())
	wr.PUT("/:id/status", h.HandleSetStatus())
	rp := g.Group("", middleware.RequirePermission(cache, "system:user:reset_pass"))
	rp.PUT("/:id/password", h.HandleSetPassword())
	del := g.Group("", middleware.RequirePermission(cache, "system:user:delete"))
	del.DELETE("/:id", h.HandleDelete())
}

func mountRole(protected *gin.RouterGroup, h *role.Handler, cache *rbac.PermissionCache) {
	g := protected.Group("/roles")
	rd := g.Group("", middleware.RequirePermission(cache, "system:role:read"))
	rd.GET("", h.HandleList())
	rd.GET("/:id", h.HandleGet())
	wr := g.Group("", middleware.RequirePermission(cache, "system:role:write"))
	wr.POST("", h.HandleCreate())
	wr.PUT("/:id", h.HandleUpdate())
	wr.PUT("/:id/permissions", h.HandleSetPermissions())
	del := g.Group("", middleware.RequirePermission(cache, "system:role:delete"))
	del.DELETE("/:id", h.HandleDelete())
}

func mountPerm(protected *gin.RouterGroup, h *permission.Handler, cache *rbac.PermissionCache) {
	g := protected.Group("/permissions")
	rd := g.Group("", middleware.RequirePermission(cache, "system:permission:read"))
	rd.GET("", h.HandleList())
}

func mountMenu(protected *gin.RouterGroup, h *menu.Handler, cache *rbac.PermissionCache) {
	g := protected.Group("/menus")
	rd := g.Group("", middleware.RequirePermission(cache, "system:menu:read"))
	rd.GET("", h.HandleTree())
	wr := g.Group("", middleware.RequirePermission(cache, "system:menu:write"))
	wr.POST("", h.HandleCreate())
	wr.PUT("/:id", h.HandleUpdate())
	del := g.Group("", middleware.RequirePermission(cache, "system:menu:delete"))
	del.DELETE("/:id", h.HandleDelete())
}

func mountAudit(protected *gin.RouterGroup, h *audit.Handler, cache *rbac.PermissionCache) {
	g := protected.Group("/audit-logs")
	rd := g.Group("", middleware.RequirePermission(cache, "system:audit:read"))
	rd.GET("", h.HandleList())
}
