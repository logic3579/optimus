//go:build dbtest

package integration_test

import (
	"context"
	"crypto/rand"
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
	"optimus-be/internal/modules/apps/application"
	appsmodule "optimus-be/internal/modules/apps/module"
	apprepo "optimus-be/internal/modules/apps/repo"
	"optimus-be/internal/modules/apps/release"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/auth"
	"optimus-be/internal/modules/credentials/vault"
	"optimus-be/internal/modules/health"
	"optimus-be/internal/modules/rbac"
	"optimus-be/internal/seed"
)

// setupAppsServer boots a fresh Postgres + the full /apps surface wired in the
// same way cmd/server/main.go does, with a real vault.Cipher (random key) and
// the in-memory helm test factory. Returns the engine, the *gorm.DB, the apps
// Module (so individual tests can patch the HelmInstalledChecker seam), and
// the in-memory helm factory whose action.Configuration backs every release
// call (one storage shared per setup -> install/upgrade/rollback see the same
// release history).
//
// Auth + RBAC are real: the seeded "admin" account with deterministic password
// "S3cret-Pass!" satisfies every /apps:* permission via the admin wildcard.
func setupAppsServer(t *testing.T) (*gin.Engine, *gorm.DB, *appsmodule.Module, *appsFakeFactory) {
	t.Helper()
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	t.Cleanup(teardown)

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{AdminUsername: "admin", AdminEmail: "a@x", BcryptCost: 4})
	require.NoError(t, err)
	hash, _ := crypto.HashPassword("S3cret-Pass!", 4)
	require.NoError(t, gdb.Model(&models.User{}).Where("username = ?", "admin").Update("password_hash", hash).Error)

	// Real vault.Cipher with a random key so the apps/repo password tri-state
	// round-trips through real AES-GCM, exactly like production.
	key := make([]byte, vault.KeyLen)
	_, err = rand.Read(key)
	require.NoError(t, err)
	cipher, err := vault.NewCipher(key)
	require.NoError(t, err)

	signer := crypto.NewJWTSigner(e2eSecret)
	authSvc := auth.NewService(
		auth.NewRepo(gdb), signer,
		ratelimit.NewLoginLimiter(50, time.Minute, time.Minute),
		auth.ServiceOptions{AccessTTL: time.Minute, RefreshTTL: time.Hour, BcryptCost: 4},
	)
	authHandler := auth.NewHandler(authSvc)

	cache := rbac.NewPermissionCache(gdb, time.Minute)
	auditRec := audit.NewRecorder(gdb)

	repoSvc := apprepo.NewService(apprepo.NewRepo(gdb), cipher, auditRec)
	appRepo := application.NewRepo(gdb)
	appSvc := application.NewService(appRepo, auditRec)
	// Cross-wire the InUseCounter so apps/repo.Delete refuses while
	// applications still reference the repo.
	repoSvc.SetInUseCounter(appRepo)

	factory := newAppsFakeFactory()
	relSvc := release.NewService(factory, appSvc, &appsmodule.HelmChartLoader{Repo: repoSvc}, auditRec)

	mod := appsmodule.New(repoSvc, appSvc, relSvc)

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
	mod.MountRoutes(protected, cache)

	return r, gdb, mod, factory
}

// authedDo issues an authenticated HTTP request against the engine returned
// by setupAppsServer.
func authedDo(t *testing.T, r *gin.Engine, bearer, method, path string, body any) *httptest.ResponseRecorder {
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

// seedClusterWithKubeconfig inserts a clusters + credentials_kubeconfigs row
// suitable for FK satisfaction. The kubeconfig bytes are NOT a real config —
// helm calls in these tests go through the in-memory fake factory and never
// touch the kubeconfig. Returns the cluster ID.
func seedClusterWithKubeconfig(t *testing.T, gdb *gorm.DB, name string) uint64 {
	t.Helper()
	kc := &models.CredentialKubeconfig{
		Name:             name + "-kc",
		DefaultNamespace: "default",
		KubeconfigEnc:    []byte("placeholder"),
	}
	require.NoError(t, gdb.Create(kc).Error)
	c := &models.Cluster{
		Name:         name,
		KubeconfigID: kc.ID,
		Context:      "test-ctx",
	}
	require.NoError(t, gdb.Create(c).Error)
	return c.ID
}

// --- TestE2E_AppsRepo_HappyPath ------------------------------------------------

// TestE2E_AppsRepo_HappyPath exercises POST/GET/LIST for chart repos and
// proves the password tri-state never leaks plaintext or ciphertext to clients
// — has_password is the only password signal callers ever see.
func TestE2E_AppsRepo_HappyPath(t *testing.T) {
	r, _, _, _ := setupAppsServer(t)
	access := login(t, r, "admin", "S3cret-Pass!")
	bearer := "Bearer " + access

	// create
	rec := authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/repos", map[string]any{
		"name": "bitnami", "type": "http", "url": "https://charts.bitnami.com/bitnami",
		"username": "u", "password": "secret",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "POST /apps/repos body=%s", rec.Body.String())
	created := bodyMap(t, rec)["data"].(map[string]any)
	repoID := uint64(created["id"].(float64))
	require.NotZero(t, repoID)
	require.Equal(t, true, created["has_password"])
	_, hasPwd := created["password"]
	require.False(t, hasPwd, "password must never appear in any response")
	_, hasEnc := created["encrypted_password"]
	require.False(t, hasEnc, "encrypted_password must never appear in any response")

	// list — same invariants must hold on the list shape too
	rec = authedDo(t, r, bearer, http.MethodGet, "/api/v1/apps/repos?page=1&page_size=20", nil)
	require.Equalf(t, http.StatusOK, rec.Code, "GET /apps/repos body=%s", rec.Body.String())
	var listResp struct {
		Code int `json:"code"`
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&listResp))
	require.Len(t, listResp.Data.Items, 1)
	row := listResp.Data.Items[0]
	require.Equal(t, true, row["has_password"])
	_, hasPwd = row["password"]
	require.False(t, hasPwd, "list rows must not include password")
	_, hasEnc = row["encrypted_password"]
	require.False(t, hasEnc, "list rows must not include encrypted_password")

	// get
	rec = authedDo(t, r, bearer, http.MethodGet, "/api/v1/apps/repos/"+itoa(repoID), nil)
	require.Equalf(t, http.StatusOK, rec.Code, "GET /apps/repos/:id body=%s", rec.Body.String())
	got := bodyMap(t, rec)["data"].(map[string]any)
	require.Equal(t, true, got["has_password"])
	_, hasPwd = got["password"]
	require.False(t, hasPwd, "detail must not include password")
}

// TestE2E_AppsRepo_UpdateNullClearsPassword proves the PUT password tri-state:
// `{"password": null}` clears has_password while leaving every other field
// intact.
func TestE2E_AppsRepo_UpdateNullClearsPassword(t *testing.T) {
	r, _, _, _ := setupAppsServer(t)
	access := login(t, r, "admin", "S3cret-Pass!")
	bearer := "Bearer " + access

	rec := authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/repos", map[string]any{
		"name": "repo-pw", "type": "http", "url": "https://x", "username": "u", "password": "secret",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "POST /apps/repos body=%s", rec.Body.String())
	repoID := uint64(bodyMap(t, rec)["data"].(map[string]any)["id"].(float64))

	// PUT with explicit null password -> clears ciphertext.
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/repos/"+itoa(repoID),
		mustJSONBody(t, json.RawMessage(`{"password": null}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearer)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equalf(t, http.StatusOK, w.Code, "PUT /apps/repos/:id body=%s", w.Body.String())

	rec = authedDo(t, r, bearer, http.MethodGet, "/api/v1/apps/repos/"+itoa(repoID), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	got := bodyMap(t, rec)["data"].(map[string]any)
	require.Equal(t, false, got["has_password"], "explicit null must clear has_password")
}

// TestE2E_AppsRepo_DeleteRefusedWhenInUse confirms apps/repo.Delete is gated
// by the InUseCounter seam: while an application row references the repo,
// DELETE returns 42002 CodeAppsChartRepoInUse.
func TestE2E_AppsRepo_DeleteRefusedWhenInUse(t *testing.T) {
	r, gdb, _, _ := setupAppsServer(t)
	access := login(t, r, "admin", "S3cret-Pass!")
	bearer := "Bearer " + access

	// Create repo + cluster + application referencing the repo.
	rec := authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/repos", map[string]any{
		"name": "in-use", "type": "http", "url": "https://x",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "POST /apps/repos body=%s", rec.Body.String())
	repoID := uint64(bodyMap(t, rec)["data"].(map[string]any)["id"].(float64))
	clusterID := seedClusterWithKubeconfig(t, gdb, "cluster-in-use")

	rec = authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/applications", map[string]any{
		"name": "app1", "cluster_id": clusterID, "namespace": "default",
		"release_name": "rel-in-use", "chart_repo_id": repoID, "chart_name": "demo",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "POST /apps/applications body=%s", rec.Body.String())

	// DELETE the repo -> refused.
	rec = authedDo(t, r, bearer, http.MethodDelete, "/api/v1/apps/repos/"+itoa(repoID), nil)
	// 42002 -> HTTPStatus is 400 (codes.go maps 40000..41000 to 400).
	require.NotEqual(t, http.StatusOK, rec.Code, "DELETE should not succeed while in use, body=%s", rec.Body.String())
	body := bodyMap(t, rec)
	require.EqualValues(t, 42002, body["code"], "expected CodeAppsChartRepoInUse, body=%s", rec.Body.String())
}

// TestE2E_AppsRepo_SoftDeletedNameReuse asserts that a soft-deleted repo's
// name can be re-used by a freshly-created repo. GORM's soft-delete adds a
// non-null deleted_at; FindByName filters on the default scope (deleted_at IS
// NULL) so the reuse must succeed.
func TestE2E_AppsRepo_SoftDeletedNameReuse(t *testing.T) {
	r, _, _, _ := setupAppsServer(t)
	access := login(t, r, "admin", "S3cret-Pass!")
	bearer := "Bearer " + access

	rec := authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/repos", map[string]any{
		"name": "reusable", "type": "http", "url": "https://x",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "POST /apps/repos body=%s", rec.Body.String())
	repoID := uint64(bodyMap(t, rec)["data"].(map[string]any)["id"].(float64))

	rec = authedDo(t, r, bearer, http.MethodDelete, "/api/v1/apps/repos/"+itoa(repoID), nil)
	require.Equalf(t, http.StatusOK, rec.Code, "DELETE /apps/repos/:id body=%s", rec.Body.String())

	// Now reuse the name.
	rec = authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/repos", map[string]any{
		"name": "reusable", "type": "http", "url": "https://y",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "POST /apps/repos (reuse) body=%s", rec.Body.String())
}
