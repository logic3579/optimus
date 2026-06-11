//go:build dbtest

package integration_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	kubefake "helm.sh/helm/v3/pkg/kube/fake"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"

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

// --- in-memory helm test factory ---------------------------------------------

// appsFakeFactory satisfies release.Factory by returning the SAME helm
// action.Configuration on every call, backed by storage.driver.Memory. Because
// the storage is shared across calls, install/upgrade/rollback/uninstall all
// see the same release history — exactly what the integration tests need to
// drive helm SDK code paths without standing up a real cluster.
type appsFakeFactory struct {
	cfg *action.Configuration
}

func newAppsFakeFactory() *appsFakeFactory {
	cfg := &action.Configuration{
		Releases:     storage.Init(driver.NewMemory()),
		KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
		Capabilities: chartutil.DefaultCapabilities,
		Log:          func(_ string, _ ...interface{}) {},
	}
	return &appsFakeFactory{cfg: cfg}
}

func (f *appsFakeFactory) NewForCluster(_ context.Context, _ uint64, _, _ string) (*action.Configuration, error) {
	return f.cfg, nil
}

// chartLoaderStub satisfies release.ChartLoader without touching a real repo.
// Returns a minimal helm chart whose metadata version mirrors the request.
type chartLoaderStub struct{}

func (chartLoaderStub) LoadChart(_ context.Context, _ uint64, _, version string) (*chart.Chart, error) {
	return loader.LoadArchive(buildTestChartTgz(version))
}

// --- minimal chart tarball builder -------------------------------------------

// buildTestChartTgz produces an in-memory chart tarball helm.loader.LoadArchive
// can parse. The directory name is fixed to "mychart" so the Chart.yaml `name`
// matches it (helm rejects mismatches).
func buildTestChartTgz(version string) io.Reader {
	const root = "mychart"
	chartYAML := "apiVersion: v2\nname: " + root + "\nversion: " + version + "\nappVersion: \"1.0\"\n"
	const valuesYAML = ""
	const templateYAML = `apiVersion: v1
kind: ConfigMap
metadata:
  name: noop
  namespace: {{ .Release.Namespace }}
data:
  marker: "integration-test"
`
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	files := []struct {
		name string
		body string
	}{
		{root + "/Chart.yaml", chartYAML},
		{root + "/values.yaml", valuesYAML},
		{root + "/templates/configmap.yaml", templateYAML},
	}
	now := time.Now()
	for _, f := range files {
		hdr := &tar.Header{
			Name:     f.name,
			Mode:     0o644,
			Size:     int64(len(f.body)),
			ModTime:  now,
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			panic(err)
		}
		if _, err := tw.Write([]byte(f.body)); err != nil {
			panic(err)
		}
	}
	if err := tw.Close(); err != nil {
		panic(err)
	}
	if err := gz.Close(); err != nil {
		panic(err)
	}
	return bytes.NewReader(buf.Bytes())
}

// --- helpers ------------------------------------------------------------------

// setupReleaseServer mirrors setupAppsServer but swaps in a chartLoaderStub
// for the chart loader so release.Install does not need real chart repo IO.
// Returns the engine, the gdb, the apps Module, and the in-memory factory
// whose helm action.Configuration backs every release call.
func setupReleaseServer(t *testing.T) (*gin.Engine, *gorm.DB, *appsmodule.Module, *appsFakeFactory) {
	t.Helper()
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	t.Cleanup(teardown)

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{AdminUsername: "admin", AdminEmail: "a@x", BcryptCost: 4})
	require.NoError(t, err)
	hash, _ := crypto.HashPassword("S3cret-Pass!", 4)
	require.NoError(t, gdb.Model(&models.User{}).Where("username = ?", "admin").Update("password_hash", hash).Error)

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
	repoSvc.SetInUseCounter(appRepo)

	factory := newAppsFakeFactory()
	// chartLoaderStub bypasses apps/repo.LoadChart entirely — release.Install
	// asks the loader for a chart, never touching the upstream URL.
	relSvc := release.NewService(factory, appSvc, chartLoaderStub{}, auditRec)

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

// makeAppForRelease seeds repo + cluster + application and returns the app ID.
func makeAppForRelease(t *testing.T, r *gin.Engine, gdb *gorm.DB, bearer, clusterName, releaseName string) uint64 {
	t.Helper()
	rec := authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/repos", map[string]any{
		"name": "repo-" + releaseName, "type": "http", "url": "https://x",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "POST /apps/repos body=%s", rec.Body.String())
	repoID := uint64(bodyMap(t, rec)["data"].(map[string]any)["id"].(float64))
	clusterID := seedClusterWithKubeconfig(t, gdb, clusterName)

	rec = authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/applications", map[string]any{
		"name": releaseName, "cluster_id": clusterID, "namespace": "default",
		"release_name": releaseName, "chart_repo_id": repoID, "chart_name": "mychart",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "POST /apps/applications body=%s", rec.Body.String())
	return uint64(bodyMap(t, rec)["data"].(map[string]any)["id"].(float64))
}

// createViewerUser builds a viewer-role-only account so RBAC denial tests can
// exercise the 40302 permission gate.
func createViewerUser(t *testing.T, gdb *gorm.DB, username, password string) {
	t.Helper()
	hash, _ := crypto.HashPassword(password, 4)
	u := &models.User{Username: username, Email: username + "@x", PasswordHash: hash, Status: "enabled"}
	require.NoError(t, gdb.Create(u).Error)
	var viewer models.Role
	require.NoError(t, gdb.Where("code = ?", "viewer").First(&viewer).Error)
	require.NoError(t, gdb.Create(&models.UserRole{UserID: u.ID, RoleID: viewer.ID}).Error)
}

// --- tests --------------------------------------------------------------------

// TestE2E_AppsRelease_WriteEndpointsRequireSpecificPermission asserts that
// each write endpoint enforces its own apps:release:* permission via the
// per-route RequirePermission middleware. The viewer role has wildcard *:read
// only, so all writes must 403.
func TestE2E_AppsRelease_WriteEndpointsRequireSpecificPermission(t *testing.T) {
	r, gdb, _, _ := setupReleaseServer(t)
	createViewerUser(t, gdb, "viewer-rel", "Viewer-Pass-1!")
	adminAccess := login(t, r, "admin", "S3cret-Pass!")
	appID := makeAppForRelease(t, r, gdb, "Bearer "+adminAccess, "cluster-rbac", "rbac-app")

	viewerAccess := login(t, r, "viewer-rel", "Viewer-Pass-1!")
	bearer := "Bearer " + viewerAccess

	cases := []struct {
		path string
		body any
	}{
		{"/api/v1/apps/applications/" + itoa(appID) + "/release/install", map[string]any{"chart_version": "1.0.0"}},
		{"/api/v1/apps/applications/" + itoa(appID) + "/release/upgrade", map[string]any{"chart_version": "1.1.0"}},
		{"/api/v1/apps/applications/" + itoa(appID) + "/release/rollback", map[string]any{"revision": 1}},
		{"/api/v1/apps/applications/" + itoa(appID) + "/release/uninstall", map[string]any{}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.path, func(t *testing.T) {
			rec := authedDo(t, r, bearer, http.MethodPost, c.path, c.body)
			require.Equalf(t, http.StatusForbidden, rec.Code, "viewer should be denied POST %s, body=%s", c.path, rec.Body.String())
			body := bodyMap(t, rec)
			require.EqualValues(t, 40302, body["code"], "expected CodeAccessDenied, body=%s", rec.Body.String())
		})
	}

	// Status / history use apps:application:read which the viewer DOES have via
	// the wildcard *:read grant. They should NOT 403 — they may return a
	// release-not-found BizError because no helm release exists yet.
	rec := authedDo(t, r, bearer, http.MethodGet,
		"/api/v1/apps/applications/"+itoa(appID)+"/release/status", nil)
	require.NotEqual(t, http.StatusForbidden, rec.Code,
		"viewer must be allowed to read release status, body=%s", rec.Body.String())
}

// TestE2E_AppsRelease_InstallWritesAudit drives helm install through the real
// HTTP surface (admin user, in-memory helm factory) and asserts exactly one
// audit row is written with Action="apps.release.install" pointing at the
// application ID.
func TestE2E_AppsRelease_InstallWritesAudit(t *testing.T) {
	r, gdb, _, _ := setupReleaseServer(t)
	access := login(t, r, "admin", "S3cret-Pass!")
	bearer := "Bearer " + access
	appID := makeAppForRelease(t, r, gdb, bearer, "cluster-aud", "aud-app")

	rec := authedDo(t, r, bearer, http.MethodPost,
		"/api/v1/apps/applications/"+itoa(appID)+"/release/install",
		map[string]any{"chart_version": "1.0.0"})
	require.Equalf(t, http.StatusOK, rec.Code, "install body=%s", rec.Body.String())
	got := bodyMap(t, rec)["data"].(map[string]any)
	require.EqualValues(t, 1, got["revision"])
	require.Equal(t, "deployed", got["status"])

	// Audit row written with Action=apps.release.install
	var rows []models.AuditLog
	require.NoError(t, gdb.Where("action = ? AND target_id = ?", "apps.release.install", itoa(appID)).
		Find(&rows).Error)
	require.Len(t, rows, 1, "expected exactly one audit row for apps.release.install")
	require.Equal(t, "apps_application", rows[0].TargetType)
	// Payload contains chart_version we sent.
	var payload map[string]any
	require.NoError(t, json.Unmarshal(rows[0].Payload, &payload))
	require.Equal(t, "1.0.0", payload["chart_version"])
}

// TestE2E_AppsRelease_FullLifecycle drives install -> upgrade -> rollback ->
// uninstall through HTTP and asserts each write writes exactly one matching
// audit row.
func TestE2E_AppsRelease_FullLifecycle(t *testing.T) {
	r, gdb, _, _ := setupReleaseServer(t)
	access := login(t, r, "admin", "S3cret-Pass!")
	bearer := "Bearer " + access
	appID := makeAppForRelease(t, r, gdb, bearer, "cluster-life", "life-app")
	base := "/api/v1/apps/applications/" + itoa(appID) + "/release"

	rec := authedDo(t, r, bearer, http.MethodPost, base+"/install",
		map[string]any{"chart_version": "1.0.0"})
	require.Equalf(t, http.StatusOK, rec.Code, "install body=%s", rec.Body.String())

	rec = authedDo(t, r, bearer, http.MethodPost, base+"/upgrade",
		map[string]any{"chart_version": "1.1.0"})
	require.Equalf(t, http.StatusOK, rec.Code, "upgrade body=%s", rec.Body.String())
	require.EqualValues(t, 2, bodyMap(t, rec)["data"].(map[string]any)["revision"])

	rec = authedDo(t, r, bearer, http.MethodPost, base+"/rollback",
		map[string]any{"revision": 1})
	require.Equalf(t, http.StatusOK, rec.Code, "rollback body=%s", rec.Body.String())
	// Helm wraps a rollback as a new release version (3 = rollback of 2 -> 1).
	require.EqualValues(t, 3, bodyMap(t, rec)["data"].(map[string]any)["revision"])

	rec = authedDo(t, r, bearer, http.MethodPost, base+"/uninstall", map[string]any{})
	require.Equalf(t, http.StatusOK, rec.Code, "uninstall body=%s", rec.Body.String())

	// Four audit rows: one per write.
	wantActions := []string{
		"apps.release.install",
		"apps.release.upgrade",
		"apps.release.rollback",
		"apps.release.uninstall",
	}
	for _, action := range wantActions {
		var rows []models.AuditLog
		require.NoError(t, gdb.Where("action = ? AND target_id = ?", action, itoa(appID)).Find(&rows).Error)
		require.Lenf(t, rows, 1, "expected exactly one audit row for %s", action)
	}
}

// TestE2E_AppsRelease_HistoryNotFound asserts GET /release/history returns a
// release-not-found BizError (42202 via apps.MapError on
// helm.ErrReleaseNotFound) when no helm release exists yet.
func TestE2E_AppsRelease_HistoryNotFound(t *testing.T) {
	r, gdb, _, _ := setupReleaseServer(t)
	access := login(t, r, "admin", "S3cret-Pass!")
	bearer := "Bearer " + access
	appID := makeAppForRelease(t, r, gdb, bearer, "cluster-h", "h-app")

	rec := authedDo(t, r, bearer, http.MethodGet,
		"/api/v1/apps/applications/"+itoa(appID)+"/release/history", nil)
	require.NotEqual(t, http.StatusOK, rec.Code, "history without install should not be OK, body=%s", rec.Body.String())
	body := bodyMap(t, rec)
	require.EqualValues(t, 42202, body["code"], "expected CodeAppsReleaseNotFound, body=%s", rec.Body.String())
}

// TestE2E_AppsRelease_InstallDuplicate asserts a second install of the same
// release returns 42201 CodeAppsReleaseAlreadyExists.
func TestE2E_AppsRelease_InstallDuplicate(t *testing.T) {
	r, gdb, _, _ := setupReleaseServer(t)
	access := login(t, r, "admin", "S3cret-Pass!")
	bearer := "Bearer " + access
	appID := makeAppForRelease(t, r, gdb, bearer, "cluster-dup", "dup-app")
	base := "/api/v1/apps/applications/" + itoa(appID) + "/release"

	rec := authedDo(t, r, bearer, http.MethodPost, base+"/install",
		map[string]any{"chart_version": "1.0.0"})
	require.Equalf(t, http.StatusOK, rec.Code, "first install body=%s", rec.Body.String())

	rec = authedDo(t, r, bearer, http.MethodPost, base+"/install",
		map[string]any{"chart_version": "1.0.0"})
	require.NotEqual(t, http.StatusOK, rec.Code, "duplicate install should fail, body=%s", rec.Body.String())
	body := bodyMap(t, rec)
	require.EqualValues(t, 42201, body["code"], "expected CodeAppsReleaseAlreadyExists, body=%s", rec.Body.String())
}

