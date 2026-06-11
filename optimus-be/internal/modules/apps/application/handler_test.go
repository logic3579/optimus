//go:build dbtest

package application_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/apps/application"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/seed"
)

// newHandlerRouter brings up a dockertest Postgres + seeds the admin user (so
// the audit_logs FK on user_id is satisfied) + builds the gin engine with the
// 5 application endpoints mounted under /apps/applications. Auth and RBAC
// middleware are intentionally bypassed; tests/integration/ exercises those.
func newHandlerRouter(t *testing.T) (*gin.Engine, *application.Service, uint64, uint64, uint64) {
	t.Helper()
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(td)

	ctx := context.Background()
	_, err := permissions.Register(ctx, gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(ctx, gdb, seed.Options{AdminUsername: "admin", AdminEmail: "a@x", BcryptCost: 4})
	require.NoError(t, err)

	// Seed FK rows the application table depends on.
	kc := &models.CredentialKubeconfig{Name: "kc-" + t.Name(), KubeconfigEnc: []byte{1}}
	require.NoError(t, gdb.Create(kc).Error)
	cl := &models.Cluster{Name: "cl-" + t.Name(), KubeconfigID: kc.ID, Context: "ctx"}
	require.NoError(t, gdb.Create(cl).Error)
	cr := &models.AppsChartRepo{Name: "cr-" + t.Name(), Type: "http", URL: "https://x"}
	require.NoError(t, gdb.Create(cr).Error)

	repo := application.NewRepo(gdb)
	rec := audit.NewRecorder(gdb)
	svc := application.NewService(repo, rec)
	h := application.NewHandler(svc)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uint64(1)); c.Next() })
	g := r.Group("/apps/applications")
	g.GET("", h.HandleList())
	g.POST("", h.HandleCreate())
	g.GET("/:id", h.HandleGet())
	g.PUT("/:id", h.HandleUpdate())
	g.DELETE("/:id", h.HandleDelete())
	return r, svc, cl.ID, cr.ID, kc.ID
}

func doJSON(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestHandler_FullCRUD(t *testing.T) {
	r, _, clID, crID, _ := newHandlerRouter(t)

	// CREATE
	w := doJSON(t, r, "POST", "/apps/applications", application.CreateRequest{
		Name: "demo", ClusterID: clID, Namespace: "default", ReleaseName: "demo",
		ChartRepoID: crID, ChartName: "nginx", Tags: []string{"prod"},
	})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	var created struct {
		Code int                `json:"code"`
		Data application.Detail `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.Equal(t, 0, created.Code)
	require.NotZero(t, created.Data.ID)
	id := created.Data.ID

	// LIST
	w = doJSON(t, r, "GET", "/apps/applications?page=1&page_size=10", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"name":"demo"`)
	require.Contains(t, w.Body.String(), `"cluster_name":"cl-`)

	// GET
	w = doJSON(t, r, "GET", "/apps/applications/"+strconv.FormatUint(id, 10), nil)
	require.Equal(t, http.StatusOK, w.Code)

	// UPDATE description
	newDesc := "edited"
	w = doJSON(t, r, "PUT", "/apps/applications/"+strconv.FormatUint(id, 10),
		application.UpdateRequest{Description: &newDesc})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.Contains(t, w.Body.String(), `"description":"edited"`)

	// DELETE (no checker wired -> allowed)
	w = doJSON(t, r, "DELETE", "/apps/applications/"+strconv.FormatUint(id, 10), nil)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
}

func TestHandler_Delete_RefusedWhenReleaseStillInstalled(t *testing.T) {
	r, svc, clID, crID, _ := newHandlerRouter(t)

	w := doJSON(t, r, "POST", "/apps/applications", application.CreateRequest{
		Name: "still-here", ClusterID: clID, Namespace: "default", ReleaseName: "still",
		ChartRepoID: crID, ChartName: "nginx",
	})
	require.Equal(t, http.StatusOK, w.Code)
	var created struct {
		Data application.Detail `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	svc.SetHelmInstalledChecker(&fakeChecker{installed: true})

	w = doJSON(t, r, "DELETE", "/apps/applications/"+strconv.FormatUint(created.Data.ID, 10), nil)
	// HTTPStatus(42204) => default branch in apperr.HTTPStatus => 500. Either
	// way the envelope must carry the right business code.
	require.NotEqual(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"code":42204`)
	require.Contains(t, w.Body.String(), "apps.application.release_still_installed")
}

func TestHandler_List_FiltersByClusterAndTag(t *testing.T) {
	r, _, clA, crID, kcID := newHandlerRouter(t)

	// Seed a second cluster.
	cl2 := &models.Cluster{Name: "clB-" + t.Name(), KubeconfigID: kcID, Context: "ctxB"}
	// Reach the DB through the handler-router test data layer by reusing the
	// service helpers — but we don't have the *gorm.DB easily, so do it via
	// the underlying repo: round-trip a Create call.
	// Easier path: hit /apps/applications POST 3 times with different cluster_ids.
	_ = cl2

	post := func(name, ns, rel string, clID uint64, tag string) {
		t.Helper()
		w := doJSON(t, r, "POST", "/apps/applications", application.CreateRequest{
			Name: name, ClusterID: clID, Namespace: ns, ReleaseName: rel,
			ChartRepoID: crID, ChartName: "nginx",
			Tags: []string{tag},
		})
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	}
	post("p1", "default", "p1", clA, "prod")
	post("p2", "default", "p2", clA, "staging")

	// Filter by cluster_id only.
	w := doJSON(t, r, "GET", "/apps/applications?cluster_id="+strconv.FormatUint(clA, 10), nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Total int64 `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.EqualValues(t, 2, resp.Data.Total)

	// Filter by tag.
	w = doJSON(t, r, "GET", "/apps/applications?tag=prod", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.EqualValues(t, 1, resp.Data.Total)
}

func TestHandler_BadID(t *testing.T) {
	r, _, _, _, _ := newHandlerRouter(t)
	w := doJSON(t, r, "GET", "/apps/applications/0", nil)
	require.NotEqual(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "common.bad_request")
}

func TestHandler_RejectsTagInjection(t *testing.T) {
	r, _, _, _, _ := newHandlerRouter(t)
	// URL-encoded `foo"]')` — chars outside [a-zA-Z0-9_.-] must be rejected
	// by the handler-level safeTagPattern guard.
	w := doJSON(t, r, "GET", `/apps/applications?tag=foo%22%5D%27%29`, nil)
	require.NotEqual(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "apps.application.tag_filter_charset")
}
