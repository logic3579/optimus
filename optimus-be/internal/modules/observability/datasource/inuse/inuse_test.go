//go:build dbtest

package inuse

import (
	"context"
	"github.com/stretchr/testify/require"
	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/tests/dbtest"
	"path/filepath"
	"testing"
)

func TestCountsOnlyActiveDatasourceReferences(t *testing.T) {
	gdb, done := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "..", "migrations"))
	t.Cleanup(done)
	u := dbtest.SeedUser(t, gdb, "counter-user")
	cred := dbtest.SeedHTTPCredential(t, gdb, u.ID, "counter-cred", "bearer")
	cluster := dbtest.SeedCluster(t, gdb, "counter-cluster")
	row := &models.ObservabilityDatasource{Name: "counter", BaseURL: "https://example.com", AuthType: "bearer", HTTPCredentialID: &cred.ID, ClusterID: &cluster.ID}
	require.NoError(t, gdb.Create(row).Error)
	c := New(gdb)
	n, err := c.CountByHTTPCredentialID(context.Background(), cred.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	n, err = c.CountByClusterID(context.Background(), cluster.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	require.NoError(t, gdb.Delete(row).Error)
	n, err = c.CountByClusterID(context.Background(), cluster.ID)
	require.NoError(t, err)
	require.Zero(t, n)
}
