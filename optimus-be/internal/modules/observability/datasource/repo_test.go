//go:build dbtest

package datasource

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/tests/dbtest"
)

func TestRepoCRUDListFiltersAndCounts(t *testing.T) {
	gdb, done := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(done)
	ctx := context.Background()
	r := NewRepo(gdb)
	user := dbtest.SeedUser(t, gdb, "ds-user")
	cred := dbtest.SeedHTTPCredential(t, gdb, user.ID, "prom-basic", "basic")
	cluster := dbtest.SeedCluster(t, gdb, "prod")
	one := &models.ObservabilityDatasource{Name: "prod-prom", BaseURL: "https://prom.example.com", AuthType: "basic", HTTPCredentialID: &cred.ID, ClusterID: &cluster.ID, CreatedByUserID: &user.ID}
	two := &models.ObservabilityDatasource{Name: "dev-prom", BaseURL: "https://dev.example.com", AuthType: "none"}
	require.NoError(t, r.Create(ctx, one))
	require.NoError(t, r.Create(ctx, two))
	items, n, err := r.List(ctx, ListQuery{Q: "prod", AuthType: "basic", ClusterID: &cluster.ID, Page: 1, PageSize: 1})
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	require.Len(t, items, 1)
	require.Equal(t, "prom-basic", items[0].HTTPCredential.Name)
	require.False(t, items[0].HasCustomCA)
	c, err := r.CountByHTTPCredentialID(ctx, cred.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, c)
	c, err = r.CountByClusterID(ctx, cluster.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, c)
	require.NoError(t, r.SoftDelete(ctx, one.ID))
	c, err = r.CountByClusterID(ctx, cluster.ID)
	require.NoError(t, err)
	require.Zero(t, c)
}

func TestRepoMapsActiveNameConflict(t *testing.T) {
	gdb, done := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(done)
	r := NewRepo(gdb)
	ctx := context.Background()
	require.NoError(t, r.Create(ctx, &models.ObservabilityDatasource{Name: "same", BaseURL: "https://a.example", AuthType: "none"}))
	err := r.Create(ctx, &models.ObservabilityDatasource{Name: "same", BaseURL: "https://b.example", AuthType: "none"})
	code(t, err, 44002)
}
