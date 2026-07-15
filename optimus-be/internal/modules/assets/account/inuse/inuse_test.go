//go:build dbtest

package inuse

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/tests/dbtest"
)

func TestGORMCounter_CountByCloudKeyID(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	ctx := context.Background()
	cloudKey := dbtest.SeedCloudKey(t, gdb, "ck")
	counter := New(gdb)

	count, err := counter.CountByCloudKeyID(ctx, cloudKey.ID)
	require.NoError(t, err)
	require.Zero(t, count)

	account := &models.CloudAccount{
		Name: "prod", Provider: "aws", CloudKeyID: cloudKey.ID,
		EnabledRegions: models.StringArray{"us-east-1"}, Enabled: true,
	}
	require.NoError(t, gdb.Create(account).Error)
	count, err = counter.CountByCloudKeyID(ctx, cloudKey.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
}

func TestGORMCounter_CountByCloudKeyID_ExcludesSoftDeleted(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	cloudKey := dbtest.SeedCloudKey(t, gdb, "ck-deleted")
	account := &models.CloudAccount{
		Name: "deleted", Provider: "aws", CloudKeyID: cloudKey.ID,
		EnabledRegions: models.StringArray{"us-east-1"}, Enabled: true,
	}
	require.NoError(t, gdb.Create(account).Error)
	require.NoError(t, gdb.Delete(account).Error)

	count, err := New(gdb).CountByCloudKeyID(context.Background(), cloudKey.ID)
	require.NoError(t, err)
	require.Zero(t, count)
}
