//go:build dbtest

package account

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/tests/dbtest"
)

func setupRepo(t *testing.T) (*Repo, *gorm.DB) {
	t.Helper()
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	return NewRepo(gdb), gdb
}

func TestRepo_CreateAndFindByID(t *testing.T) {
	r, gdb := setupRepo(t)
	ctx := context.Background()
	ck := dbtest.SeedCloudKey(t, gdb, "k1")

	row := &models.CloudAccount{
		Name: "prod-aws", Provider: "aws", CloudKeyID: ck.ID,
		EnabledRegions: models.StringArray{"us-east-1", "ap-northeast-1"},
		Enabled:        true,
	}
	require.NoError(t, r.Create(ctx, row))
	require.NotZero(t, row.ID)

	got, err := r.FindByID(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, "prod-aws", got.Name)
	require.Len(t, got.EnabledRegions, 2)
}

func TestRepo_NameConflictOnAlive(t *testing.T) {
	r, gdb := setupRepo(t)
	ctx := context.Background()
	ck := dbtest.SeedCloudKey(t, gdb, "k1")
	a := &models.CloudAccount{Name: "x", Provider: "aws", CloudKeyID: ck.ID, EnabledRegions: models.StringArray{"us-east-1"}}
	require.NoError(t, r.Create(ctx, a))
	b := &models.CloudAccount{Name: "x", Provider: "aws", CloudKeyID: ck.ID, EnabledRegions: models.StringArray{"us-east-1"}}
	require.Error(t, r.Create(ctx, b))
}

func TestRepo_NameReusableAfterSoftDelete(t *testing.T) {
	r, gdb := setupRepo(t)
	ctx := context.Background()
	ck := dbtest.SeedCloudKey(t, gdb, "k1")
	a := &models.CloudAccount{Name: "x", Provider: "aws", CloudKeyID: ck.ID, EnabledRegions: models.StringArray{"us-east-1"}}
	require.NoError(t, r.Create(ctx, a))
	require.NoError(t, r.SoftDelete(ctx, a.ID))
	b := &models.CloudAccount{Name: "x", Provider: "aws", CloudKeyID: ck.ID, EnabledRegions: models.StringArray{"us-east-1"}}
	require.NoError(t, r.Create(ctx, b))
}

func TestRepo_CascadeSoftDelete_AllRegions(t *testing.T) {
	r, gdb := setupRepo(t)
	ctx := context.Background()
	ck := dbtest.SeedCloudKey(t, gdb, "k1")
	a := &models.CloudAccount{Name: "p", Provider: "aws", CloudKeyID: ck.ID, EnabledRegions: models.StringArray{"us-east-1", "us-west-2"}}
	require.NoError(t, r.Create(ctx, a))
	dbtest.SeedAWSInstance(t, gdb, a.ID, "us-east-1", "i-a")
	dbtest.SeedAWSInstance(t, gdb, a.ID, "us-west-2", "i-b")
	n, err := r.CascadeSoftDeleteResources(ctx, nil, a.ID, nil)
	require.NoError(t, err)
	require.EqualValues(t, 2, n)
}

func TestRepo_CascadeSoftDelete_SubsetRegions(t *testing.T) {
	r, gdb := setupRepo(t)
	ctx := context.Background()
	ck := dbtest.SeedCloudKey(t, gdb, "k1")
	a := &models.CloudAccount{Name: "p", Provider: "aws", CloudKeyID: ck.ID, EnabledRegions: models.StringArray{"us-east-1", "us-west-2"}}
	require.NoError(t, r.Create(ctx, a))
	dbtest.SeedAWSInstance(t, gdb, a.ID, "us-east-1", "i-a")
	dbtest.SeedAWSInstance(t, gdb, a.ID, "us-west-2", "i-b")
	n, err := r.CascadeSoftDeleteResources(ctx, nil, a.ID, []string{"us-west-2"})
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	var alive int64
	require.NoError(t, gdb.Model(&models.AWSInstance{}).Where("cloud_account_id = ?", a.ID).Count(&alive).Error)
	require.EqualValues(t, 1, alive)
}
