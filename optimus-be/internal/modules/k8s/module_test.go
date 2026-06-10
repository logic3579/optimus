//go:build dbtest

package k8s_test

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/k8s"
	"optimus-be/internal/modules/rbac"
)

// TestMountRoutes_Snapshot guards the 21-route surface of /api/v1/k8s
// against silent drift — adding, renaming, or removing a route here forces
// the FE / docs / permission story to be re-checked.
//
// Uses dockertest Postgres rather than SQLite because the Cluster model's
// Tags column declares `type:jsonb`, which SQLite's AutoMigrate cannot
// honour. The test only exercises route assembly; no DB writes happen.
func TestMountRoutes_Snapshot(t *testing.T) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	t.Cleanup(td)

	cache := rbac.NewPermissionCache(gdb, time.Minute)
	m := k8s.New(gdb, &nopConsumer{}, audit.NewRecorder(gdb), cache)

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
		"DELETE /api/v1/k8s/clusters/:id",
		"GET /api/v1/k8s/clusters",
		"GET /api/v1/k8s/clusters/:id",
		"GET /api/v1/k8s/clusters/:id/config/configmaps",
		"GET /api/v1/k8s/clusters/:id/config/configmaps/:ns/:name",
		"GET /api/v1/k8s/clusters/:id/events",
		"GET /api/v1/k8s/clusters/:id/namespaces",
		"GET /api/v1/k8s/clusters/:id/network/:kind",
		"GET /api/v1/k8s/clusters/:id/network/:kind/:ns/:name",
		"GET /api/v1/k8s/clusters/:id/nodes",
		"GET /api/v1/k8s/clusters/:id/nodes/:name",
		"GET /api/v1/k8s/clusters/:id/pods/:ns/:name/log",
		"GET /api/v1/k8s/clusters/:id/secrets",
		"GET /api/v1/k8s/clusters/:id/secrets/:ns/:name",
		"GET /api/v1/k8s/clusters/:id/secrets/:ns/:name/data",
		"GET /api/v1/k8s/clusters/:id/workloads/:kind",
		"GET /api/v1/k8s/clusters/:id/workloads/:kind/:ns/:name",
		"GET /api/v1/k8s/clusters/:id/yaml",
		"POST /api/v1/k8s/clusters",
		"POST /api/v1/k8s/clusters/:id/ping",
		"PUT /api/v1/k8s/clusters/:id",
	}
	require.Equal(t, want, paths, "route surface drifted:\n  got: %s", strings.Join(paths, "\n       "))
}

// nopConsumer is a never-called Consumer — the snapshot test exercises only
// route assembly, never invokes a handler, so all three methods return zero
// values. Real wiring uses the credentials module's Consumer.
type nopConsumer struct{}

func (nopConsumer) GetSSHKey(context.Context, uint64, string) (*credentials.SSHKey, error) {
	return nil, nil
}
func (nopConsumer) GetKubeconfig(context.Context, uint64, string) (*credentials.Kubeconfig, error) {
	return nil, nil
}
func (nopConsumer) GetCloudKey(context.Context, uint64, string) (*credentials.CloudKey, error) {
	return nil, nil
}
