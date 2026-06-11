//go:build dbtest

package integration_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/models"
	"optimus-be/internal/modules/apps/application"
)

// fakeHelmInstalled is a tiny stand-in for application.HelmInstalledChecker;
// tests flip Installed/Err and re-inject through Module.Application
// .SetHelmInstalledChecker right before calling DELETE.
type fakeHelmInstalled struct {
	Installed bool
	Err       error
}

func (f *fakeHelmInstalled) IsReleaseInstalled(_ context.Context, _ *models.AppsApplication) (bool, error) {
	return f.Installed, f.Err
}

// --- TestE2E_AppsApplication_HappyPath ----------------------------------------

// TestE2E_AppsApplication_HappyPath exercises POST/GET/LIST. Verifies that
// the preload-derived cluster_name + owner_name are populated on Get.
func TestE2E_AppsApplication_HappyPath(t *testing.T) {
	r, gdb, _, _ := setupAppsServer(t)
	access := login(t, r, "admin", "S3cret-Pass!")
	bearer := "Bearer " + access

	rec := authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/repos", map[string]any{
		"name": "r1", "type": "http", "url": "https://x",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "POST /apps/repos body=%s", rec.Body.String())
	repoID := uint64(bodyMap(t, rec)["data"].(map[string]any)["id"].(float64))
	clusterID := seedClusterWithKubeconfig(t, gdb, "cluster-app-happy")

	// Resolve the seeded admin user ID so owner_name decoration kicks in.
	var adminUser models.User
	require.NoError(t, gdb.Where("username = ?", "admin").First(&adminUser).Error)

	rec = authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/applications", map[string]any{
		"name": "demo-app", "cluster_id": clusterID, "namespace": "default",
		"release_name": "demo", "chart_repo_id": repoID, "chart_name": "mychart",
		"description": "hello", "tags": []string{"prod"}, "owner_user_id": adminUser.ID,
	})
	require.Equalf(t, http.StatusOK, rec.Code, "POST /apps/applications body=%s", rec.Body.String())
	created := bodyMap(t, rec)["data"].(map[string]any)
	appID := uint64(created["id"].(float64))
	require.NotZero(t, appID)
	require.Equal(t, "cluster-app-happy", created["cluster_name"])
	require.NotEmpty(t, created["owner_name"], "owner_name must be preloaded")

	// list
	rec = authedDo(t, r, bearer, http.MethodGet, "/api/v1/apps/applications?page=1&page_size=20", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	list := bodyMap(t, rec)["data"].(map[string]any)
	items := list["items"].([]any)
	require.Len(t, items, 1)

	// get
	rec = authedDo(t, r, bearer, http.MethodGet, "/api/v1/apps/applications/"+itoa(appID), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	got := bodyMap(t, rec)["data"].(map[string]any)
	require.Equal(t, "cluster-app-happy", got["cluster_name"])
}

// TestE2E_AppsApplication_DuplicateReleaseTuple verifies the
// (cluster_id, namespace, release_name) uniqueness gate returns 42003.
func TestE2E_AppsApplication_DuplicateReleaseTuple(t *testing.T) {
	r, gdb, _, _ := setupAppsServer(t)
	access := login(t, r, "admin", "S3cret-Pass!")
	bearer := "Bearer " + access

	rec := authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/repos", map[string]any{
		"name": "r-dup", "type": "http", "url": "https://x",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	repoID := uint64(bodyMap(t, rec)["data"].(map[string]any)["id"].(float64))
	clusterID := seedClusterWithKubeconfig(t, gdb, "cluster-dup")

	mk := func(name string) map[string]any {
		return map[string]any{
			"name": name, "cluster_id": clusterID, "namespace": "ns1",
			"release_name": "shared", "chart_repo_id": repoID, "chart_name": "c",
		}
	}
	rec = authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/applications", mk("app-a"))
	require.Equalf(t, http.StatusOK, rec.Code, "first POST body=%s", rec.Body.String())

	rec = authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/applications", mk("app-b"))
	require.NotEqual(t, http.StatusOK, rec.Code, "duplicate tuple should fail, body=%s", rec.Body.String())
	body := bodyMap(t, rec)
	require.EqualValues(t, 42003, body["code"], "expected CodeAppsReleaseNameDuplicate, body=%s", rec.Body.String())
}

// TestE2E_AppsApplication_CreateNonexistentCluster confirms that POSTing with
// a cluster_id that has no row produces a friendly BizError instead of a 500
// or a raw FK message leaking to the client. The service does not pre-check
// cluster existence; instead the FK violation surfaces via repo.Create and
// reaches response.Error, which logs and returns CodeInternal (50000). We
// assert the response is structured (Envelope shape) and never contains raw
// "violates foreign key" text.
func TestE2E_AppsApplication_CreateNonexistentCluster(t *testing.T) {
	r, _, _, _ := setupAppsServer(t)
	access := login(t, r, "admin", "S3cret-Pass!")
	bearer := "Bearer " + access

	rec := authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/repos", map[string]any{
		"name": "r-no-cluster", "type": "http", "url": "https://x",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	repoID := uint64(bodyMap(t, rec)["data"].(map[string]any)["id"].(float64))

	rec = authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/applications", map[string]any{
		"name": "orphan", "cluster_id": 999999, "namespace": "default",
		"release_name": "orphan", "chart_repo_id": repoID, "chart_name": "c",
	})
	require.NotEqual(t, http.StatusOK, rec.Code, "create against missing cluster should not succeed, body=%s", rec.Body.String())
	// Body must be the envelope; we don't pin the exact code because that
	// depends on whether the FK is enforced (migrations 00010 add FKs) — both
	// CodeInternal (10000) and a BizError-mapped 4xx are acceptable. What we
	// MUST NOT see is raw postgres text.
	require.NotContains(t, rec.Body.String(), "violates foreign key")
	require.NotContains(t, strings.ToLower(rec.Body.String()), "sqlstate")
	body := bodyMap(t, rec)
	_, hasCode := body["code"]
	require.True(t, hasCode, "envelope must have code field")
}

// TestE2E_AppsApplication_DeleteRefusedWhenHelmInstalled verifies the
// HelmInstalledChecker seam: when it reports true, DELETE returns 42204
// CodeAppsReleaseStillPresent.
func TestE2E_AppsApplication_DeleteRefusedWhenHelmInstalled(t *testing.T) {
	r, gdb, mod, _ := setupAppsServer(t)
	access := login(t, r, "admin", "S3cret-Pass!")
	bearer := "Bearer " + access

	rec := authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/repos", map[string]any{
		"name": "r-del", "type": "http", "url": "https://x",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	repoID := uint64(bodyMap(t, rec)["data"].(map[string]any)["id"].(float64))
	clusterID := seedClusterWithKubeconfig(t, gdb, "cluster-del")

	rec = authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/applications", map[string]any{
		"name": "still-installed", "cluster_id": clusterID, "namespace": "default",
		"release_name": "still", "chart_repo_id": repoID, "chart_name": "c",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	appID := uint64(bodyMap(t, rec)["data"].(map[string]any)["id"].(float64))

	// Inject a checker that reports the release is still installed.
	mod.Application.SetHelmInstalledChecker(&fakeHelmInstalled{Installed: true})

	rec = authedDo(t, r, bearer, http.MethodDelete, "/api/v1/apps/applications/"+itoa(appID), nil)
	require.NotEqual(t, http.StatusOK, rec.Code, "DELETE should be refused, body=%s", rec.Body.String())
	body := bodyMap(t, rec)
	require.EqualValues(t, 42204, body["code"], "expected CodeAppsReleaseStillPresent, body=%s", rec.Body.String())

	// Now flip the checker to report not installed -> DELETE succeeds.
	mod.Application.SetHelmInstalledChecker(&fakeHelmInstalled{Installed: false})
	rec = authedDo(t, r, bearer, http.MethodDelete, "/api/v1/apps/applications/"+itoa(appID), nil)
	require.Equalf(t, http.StatusOK, rec.Code, "DELETE after checker flip body=%s", rec.Body.String())
}

// TestE2E_AppsApplication_ListFilters verifies the cluster_id and tag list
// filters return the expected subset.
func TestE2E_AppsApplication_ListFilters(t *testing.T) {
	r, gdb, _, _ := setupAppsServer(t)
	access := login(t, r, "admin", "S3cret-Pass!")
	bearer := "Bearer " + access

	rec := authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/repos", map[string]any{
		"name": "r-filt", "type": "http", "url": "https://x",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	repoID := uint64(bodyMap(t, rec)["data"].(map[string]any)["id"].(float64))

	clusterA := seedClusterWithKubeconfig(t, gdb, "cluster-a")
	clusterB := seedClusterWithKubeconfig(t, gdb, "cluster-b")

	mk := func(name string, clusterID uint64, tags []string) map[string]any {
		return map[string]any{
			"name": name, "cluster_id": clusterID, "namespace": "default",
			"release_name": name, "chart_repo_id": repoID, "chart_name": "c",
			"tags": tags,
		}
	}
	rec = authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/applications", mk("a1", clusterA, []string{"prod"}))
	require.Equal(t, http.StatusOK, rec.Code)
	rec = authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/applications", mk("a2", clusterA, []string{"staging"}))
	require.Equal(t, http.StatusOK, rec.Code)
	rec = authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/applications", mk("b1", clusterB, []string{"prod"}))
	require.Equal(t, http.StatusOK, rec.Code)

	// cluster_id=clusterA -> 2
	rec = authedDo(t, r, bearer, http.MethodGet, "/api/v1/apps/applications?cluster_id="+itoa(clusterA), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	list := bodyMap(t, rec)["data"].(map[string]any)
	require.EqualValues(t, 2, list["total"])

	// tag=prod -> 2 (a1 + b1)
	rec = authedDo(t, r, bearer, http.MethodGet, "/api/v1/apps/applications?tag=prod", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	list = bodyMap(t, rec)["data"].(map[string]any)
	require.EqualValues(t, 2, list["total"])

	// cluster_id=clusterA AND tag=prod -> 1 (a1)
	rec = authedDo(t, r, bearer, http.MethodGet, "/api/v1/apps/applications?cluster_id="+itoa(clusterA)+"&tag=prod", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	list = bodyMap(t, rec)["data"].(map[string]any)
	require.EqualValues(t, 1, list["total"])
}

// TestE2E_AppsApplication_UpdateIgnoresImmutableFields confirms that PUT only
// mutates description / tags / owner_user_id; sending cluster_id, namespace,
// release_name, chart_repo_id, chart_name on the PUT body is silently dropped
// because UpdateRequest doesn't expose them.
func TestE2E_AppsApplication_UpdateIgnoresImmutableFields(t *testing.T) {
	r, gdb, _, _ := setupAppsServer(t)
	access := login(t, r, "admin", "S3cret-Pass!")
	bearer := "Bearer " + access

	rec := authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/repos", map[string]any{
		"name": "r-imm", "type": "http", "url": "https://x",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	repoID := uint64(bodyMap(t, rec)["data"].(map[string]any)["id"].(float64))
	clusterID := seedClusterWithKubeconfig(t, gdb, "cluster-imm")
	otherClusterID := seedClusterWithKubeconfig(t, gdb, "cluster-imm-other")

	rec = authedDo(t, r, bearer, http.MethodPost, "/api/v1/apps/applications", map[string]any{
		"name": "imm-app", "cluster_id": clusterID, "namespace": "default",
		"release_name": "imm", "chart_repo_id": repoID, "chart_name": "c",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	appID := uint64(bodyMap(t, rec)["data"].(map[string]any)["id"].(float64))

	// PUT with cluster_id + namespace + chart_name -> all silently ignored.
	// description + tags should still apply.
	rec = authedDo(t, r, bearer, http.MethodPut, "/api/v1/apps/applications/"+itoa(appID), map[string]any{
		"cluster_id": otherClusterID, "namespace": "tampered",
		"release_name": "tampered", "chart_name": "tampered",
		"description": "updated", "tags": []string{"v2"},
	})
	require.Equalf(t, http.StatusOK, rec.Code, "PUT body=%s", rec.Body.String())

	// Read back the DB row directly — handler response is also fine but a DB
	// read removes any doubt about serialization.
	var m models.AppsApplication
	require.NoError(t, gdb.First(&m, appID).Error)
	require.Equal(t, clusterID, m.ClusterID, "cluster_id must be immutable via PUT")
	require.Equal(t, "default", m.Namespace, "namespace must be immutable via PUT")
	require.Equal(t, "imm", m.ReleaseName, "release_name must be immutable via PUT")
	require.Equal(t, "c", m.ChartName, "chart_name must be immutable via PUT")
	require.Equal(t, "updated", m.Description)
	require.Equal(t, []string{"v2"}, []string(m.Tags))
}

// Compile-time check that application.HelmInstalledChecker is satisfied by
// our fake (catches signature drift early).
var _ application.HelmInstalledChecker = (*fakeHelmInstalled)(nil)
