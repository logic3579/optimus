//go:build dbtest

package application_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/apps/application"
)

// migrationsPath is the relative path from this test package to the embedded
// SQL migrations directory.
const migrationsPath = "../../../../migrations"

// newRepo brings up a fresh dockertest Postgres + applies migrations and
// returns the repo bound to it. The kubeconfig + cluster + chart-repo rows
// the FKs require are NOT created here — call seedFKs first.
func newRepo(t *testing.T) (*application.Repo, func()) {
	gdb, td := db.StartTestPostgres(t, filepath.Join(migrationsPath))
	return application.NewRepo(gdb), td
}

// seedFKs inserts one kubeconfig, one cluster, one chart repo and returns
// (clusterID, chartRepoID). All application rows in the same test reference
// these.
func seedFKs(t *testing.T, r *application.Repo) (uint64, uint64) {
	t.Helper()
	kc := &models.CredentialKubeconfig{Name: "kc-" + t.Name(), KubeconfigEnc: []byte{1}}
	require.NoError(t, r.DB().Create(kc).Error)
	cl := &models.Cluster{Name: "cl-" + t.Name(), KubeconfigID: kc.ID, Context: "ctx"}
	require.NoError(t, r.DB().Create(cl).Error)
	cr := &models.AppsChartRepo{Name: "cr-" + t.Name(), Type: "http", URL: "https://x"}
	require.NoError(t, r.DB().Create(cr).Error)
	return cl.ID, cr.ID
}

func TestRepo_CRUD(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	clID, crID := seedFKs(t, r)
	ctx := context.Background()

	m := &models.AppsApplication{
		Name: "demo", ClusterID: clID, Namespace: "default", ReleaseName: "demo",
		ChartRepoID: crID, ChartName: "nginx",
		Tags: datatypes.NewJSONSlice([]string{"alpha"}),
	}
	require.NoError(t, r.Create(ctx, m))
	require.NotZero(t, m.ID)

	got, err := r.Get(ctx, m.ID)
	require.NoError(t, err)
	require.Equal(t, "demo", got.Name)
	require.NotNil(t, got.Cluster)
	require.NotNil(t, got.ChartRepo)
	require.Equal(t, []string{"alpha"}, []string(got.Tags))

	_, total, err := r.List(ctx, application.ListQuery{})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)

	require.NoError(t, r.Update(ctx, m.ID, map[string]any{"description": "primary"}))
	got, err = r.Get(ctx, m.ID)
	require.NoError(t, err)
	require.Equal(t, "primary", got.Description)

	require.NoError(t, r.Delete(ctx, m.ID))
	_, err = r.Get(ctx, m.ID)
	require.Error(t, err)
}

func TestRepo_ReleaseTupleUniquePartialIndex(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	clID, crID := seedFKs(t, r)
	ctx := context.Background()

	require.NoError(t, r.Create(ctx, &models.AppsApplication{
		Name: "n1", ClusterID: clID, Namespace: "default", ReleaseName: "rel",
		ChartRepoID: crID, ChartName: "nginx",
	}))
	err := r.Create(ctx, &models.AppsApplication{
		Name: "n2", ClusterID: clID, Namespace: "default", ReleaseName: "rel",
		ChartRepoID: crID, ChartName: "nginx",
	})
	require.Error(t, err, "second insert with same (cluster_id, namespace, release_name) triple must violate partial unique index")
}

func TestRepo_ReleaseTuple_AllowedAfterSoftDelete(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	clID, crID := seedFKs(t, r)
	ctx := context.Background()

	first := &models.AppsApplication{
		Name: "first", ClusterID: clID, Namespace: "default", ReleaseName: "rel",
		ChartRepoID: crID, ChartName: "nginx",
	}
	require.NoError(t, r.Create(ctx, first))
	require.NoError(t, r.Delete(ctx, first.ID)) // soft-delete

	// Same tuple, new row — allowed because the unique index is partial on deleted_at IS NULL.
	require.NoError(t, r.Create(ctx, &models.AppsApplication{
		Name: "second", ClusterID: clID, Namespace: "default", ReleaseName: "rel",
		ChartRepoID: crID, ChartName: "nginx",
	}))
}

func TestRepo_CountByClusterID_ExcludesSoftDeleted(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	clID, crID := seedFKs(t, r)
	ctx := context.Background()

	n, err := r.CountByClusterID(ctx, clID)
	require.NoError(t, err)
	require.Equal(t, 0, n)

	a := &models.AppsApplication{
		Name: "a", ClusterID: clID, Namespace: "default", ReleaseName: "a",
		ChartRepoID: crID, ChartName: "nginx",
	}
	require.NoError(t, r.Create(ctx, a))
	b := &models.AppsApplication{
		Name: "b", ClusterID: clID, Namespace: "default", ReleaseName: "b",
		ChartRepoID: crID, ChartName: "nginx",
	}
	require.NoError(t, r.Create(ctx, b))

	n, err = r.CountByClusterID(ctx, clID)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	// Soft-delete one — must drop the count.
	require.NoError(t, r.Delete(ctx, a.ID))
	n, err = r.CountByClusterID(ctx, clID)
	require.NoError(t, err)
	require.Equal(t, 1, n)
}

func TestRepo_CountByChartRepoID_ExcludesSoftDeleted(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	clID, crID := seedFKs(t, r)
	ctx := context.Background()

	n, err := r.CountByChartRepoID(ctx, crID)
	require.NoError(t, err)
	require.Equal(t, 0, n)

	a := &models.AppsApplication{
		Name: "ca", ClusterID: clID, Namespace: "default", ReleaseName: "ca",
		ChartRepoID: crID, ChartName: "nginx",
	}
	require.NoError(t, r.Create(ctx, a))
	n, err = r.CountByChartRepoID(ctx, crID)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	require.NoError(t, r.Delete(ctx, a.ID))
	n, err = r.CountByChartRepoID(ctx, crID)
	require.NoError(t, err)
	require.Equal(t, 0, n)
}

func TestRepo_List_FiltersByClusterAndTag(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	clA, crID := seedFKs(t, r)

	// Add a second cluster on the same kubeconfig so we can filter by cluster_id.
	var kcID uint64
	require.NoError(t, r.DB().Raw(`SELECT kubeconfig_id FROM clusters WHERE id = ?`, clA).Scan(&kcID).Error)
	clB := &models.Cluster{Name: "clB-" + t.Name(), KubeconfigID: kcID, Context: "ctxB"}
	require.NoError(t, r.DB().Create(clB).Error)

	ctx := context.Background()
	require.NoError(t, r.Create(ctx, &models.AppsApplication{
		Name: "p1", ClusterID: clA, Namespace: "default", ReleaseName: "p1",
		ChartRepoID: crID, ChartName: "nginx",
		Tags: datatypes.NewJSONSlice([]string{"prod"}),
	}))
	require.NoError(t, r.Create(ctx, &models.AppsApplication{
		Name: "p2", ClusterID: clA, Namespace: "default", ReleaseName: "p2",
		ChartRepoID: crID, ChartName: "nginx",
		Tags: datatypes.NewJSONSlice([]string{"staging"}),
	}))
	require.NoError(t, r.Create(ctx, &models.AppsApplication{
		Name: "p3", ClusterID: clB.ID, Namespace: "default", ReleaseName: "p3",
		ChartRepoID: crID, ChartName: "nginx",
		Tags: datatypes.NewJSONSlice([]string{"prod"}),
	}))

	rows, total, err := r.List(ctx, application.ListQuery{ClusterID: clA})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, rows, 2)

	rows, total, err = r.List(ctx, application.ListQuery{Tag: "prod"})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, rows, 2)

	rows, total, err = r.List(ctx, application.ListQuery{ClusterID: clB.ID, Tag: "prod"})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, "p3", rows[0].Name)
}
