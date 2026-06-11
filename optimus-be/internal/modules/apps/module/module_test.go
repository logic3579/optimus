//go:build dbtest

package module_test

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/action"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/modules/apps/application"
	appsmodule "optimus-be/internal/modules/apps/module"
	apprepo "optimus-be/internal/modules/apps/repo"
	"optimus-be/internal/modules/apps/release"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/rbac"
)

// nopCipher is a never-called Cipher stub — the snapshot test exercises
// only route assembly, never reaches a handler that touches the cipher.
type nopCipher struct{}

func (nopCipher) Seal(b []byte) ([]byte, error) { return b, nil }
func (nopCipher) Open(b []byte) ([]byte, error) { return b, nil }

// nopFactory is a never-called release.Factory stub. release.NewService
// refuses a nil Factory at construction time, so we satisfy the interface
// without spinning up real helm wiring.
type nopFactory struct{}

func (nopFactory) NewForCluster(context.Context, uint64, string, string) (*action.Configuration, error) {
	return nil, nil
}

// TestMountRoutes_Snapshot guards the /apps surface against silent drift.
// Adding, renaming, or removing a route forces the FE / docs / permission
// story to be re-checked.
//
// Uses dockertest Postgres because the apps_applications model's `tags`
// column declares `type:jsonb`, which SQLite's AutoMigrate cannot honour.
// The test only exercises route assembly; no DB writes happen.
func TestMountRoutes_Snapshot(t *testing.T) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(td)

	rec := audit.NewRecorder(gdb)

	repoSvc := apprepo.NewService(apprepo.NewRepo(gdb), nopCipher{}, rec)
	appRepo := application.NewRepo(gdb)
	appSvc := application.NewService(appRepo, rec)
	relSvc := release.NewService(nopFactory{}, appSvc, &appsmodule.HelmChartLoader{Repo: repoSvc}, rec)

	m := appsmodule.New(repoSvc, appSvc, relSvc)

	cache := rbac.NewPermissionCache(gdb, time.Minute)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("/api/v1")
	m.MountRoutes(grp, cache)

	paths := make([]string, 0, len(r.Routes()))
	for _, ri := range r.Routes() {
		paths = append(paths, ri.Method+" "+ri.Path)
	}
	sort.Strings(paths)

	want := []string{
		"DELETE /api/v1/apps/applications/:id",
		"DELETE /api/v1/apps/repos/:id",
		"GET /api/v1/apps/applications",
		"GET /api/v1/apps/applications/:id",
		"GET /api/v1/apps/applications/:id/release/history",
		"GET /api/v1/apps/applications/:id/release/status",
		"GET /api/v1/apps/repos",
		"GET /api/v1/apps/repos/:id",
		"GET /api/v1/apps/repos/:id/charts",
		"GET /api/v1/apps/repos/:id/charts/:chart/versions",
		"GET /api/v1/apps/repos/:id/charts/:chart/versions/:version/values",
		"POST /api/v1/apps/applications",
		"POST /api/v1/apps/applications/:id/release/install",
		"POST /api/v1/apps/applications/:id/release/rollback",
		"POST /api/v1/apps/applications/:id/release/uninstall",
		"POST /api/v1/apps/applications/:id/release/upgrade",
		"POST /api/v1/apps/repos",
		"PUT /api/v1/apps/applications/:id",
		"PUT /api/v1/apps/repos/:id",
	}
	require.Equal(t, want, paths, "route surface drifted:\n  got: %s", strings.Join(paths, "\n       "))
}
