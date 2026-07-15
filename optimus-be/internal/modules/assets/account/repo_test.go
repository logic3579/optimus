//go:build dbtest

package account

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/assets/errs"
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

func TestRepo_NameUniqueViolationMappedOnCreateAndUpdate(t *testing.T) {
	r, gdb := setupRepo(t)
	ctx := context.Background()
	key := dbtest.SeedCloudKey(t, gdb, "conflict-key")
	first := &models.CloudAccount{Name: "first", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: models.StringArray{"us-east-1"}}
	second := &models.CloudAccount{Name: "second", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: models.StringArray{"us-east-1"}}
	require.NoError(t, r.Create(ctx, first))
	require.NoError(t, r.Create(ctx, second))
	assertNameConflict := func(err error) {
		t.Helper()
		bizErr, ok := apperr.AsBiz(err)
		require.True(t, ok)
		require.Equal(t, errs.CodeAssetsCloudAccountNameConflict, bizErr.Code)
	}
	duplicate := &models.CloudAccount{Name: "first", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: models.StringArray{"us-east-1"}}
	assertNameConflict(r.Create(ctx, duplicate))
	assertNameConflict(r.Update(ctx, second.ID, map[string]any{"name": "first"}))
}

func TestRepo_CascadeSoftDelete_AllRegions(t *testing.T) {
	r, gdb := setupRepo(t)
	ctx := context.Background()
	ck := dbtest.SeedCloudKey(t, gdb, "k1")
	a := &models.CloudAccount{Name: "p", Provider: "aws", CloudKeyID: ck.ID, EnabledRegions: models.StringArray{"us-east-1", "us-west-2"}}
	require.NoError(t, r.Create(ctx, a))
	seedResourceSet(t, gdb, a.ID, "us-east-1", "east")
	seedResourceSet(t, gdb, a.ID, "us-west-2", "west")
	n, err := r.CascadeSoftDeleteResources(ctx, nil, a.ID, nil)
	require.NoError(t, err)
	require.EqualValues(t, 8, n)
}

func TestRepo_CascadeSoftDelete_SubsetRegions(t *testing.T) {
	r, gdb := setupRepo(t)
	ctx := context.Background()
	ck := dbtest.SeedCloudKey(t, gdb, "k1")
	a := &models.CloudAccount{Name: "p", Provider: "aws", CloudKeyID: ck.ID, EnabledRegions: models.StringArray{"us-east-1", "us-west-2"}}
	require.NoError(t, r.Create(ctx, a))
	seedResourceSet(t, gdb, a.ID, "us-east-1", "east")
	seedResourceSet(t, gdb, a.ID, "us-west-2", "west")
	n, err := r.CascadeSoftDeleteResources(ctx, nil, a.ID, []string{"us-west-2"})
	require.NoError(t, err)
	require.EqualValues(t, 4, n)

	var alive int64
	require.NoError(t, gdb.Model(&models.AWSInstance{}).Where("cloud_account_id = ?", a.ID).Count(&alive).Error)
	require.EqualValues(t, 1, alive)
}

func TestRepo_ListFiltersPaginationAndDeleted(t *testing.T) {
	r, gdb := setupRepo(t)
	ctx := context.Background()
	key := dbtest.SeedCloudKey(t, gdb, "list-key")
	rows := []*models.CloudAccount{
		{Name: "prod-a", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: models.StringArray{"us-east-1"}, Enabled: true},
		{Name: "prod-b", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: models.StringArray{"us-west-2"}, Enabled: false},
		{Name: "dev", Provider: "gcp", CloudKeyID: key.ID, EnabledRegions: models.StringArray{"us-east-1"}, Enabled: true},
	}
	for _, row := range rows {
		require.NoError(t, r.Create(ctx, row))
	}
	require.NoError(t, r.SoftDelete(ctx, rows[2].ID))
	enabled := true
	items, total, err := r.List(ctx, ListQuery{Q: "prod", Provider: "aws", Enabled: &enabled, Page: 1, Size: 1})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, "prod-a", items[0].Name)
	items, total, err = r.List(ctx, ListQuery{IncludeDeleted: true, Page: 2, Size: 2})
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.Len(t, items, 1)
}

func TestRepo_UpdateCountAndCloudKeyNames(t *testing.T) {
	r, gdb := setupRepo(t)
	ctx := context.Background()
	key := dbtest.SeedCloudKey(t, gdb, "primary-key")
	otherKey := dbtest.SeedCloudKey(t, gdb, "other-key")
	row := &models.CloudAccount{Name: "old", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: models.StringArray{"us-east-1"}, Enabled: true}
	require.NoError(t, r.Create(ctx, row))
	require.NoError(t, r.Update(ctx, row.ID, map[string]any{"name": "new", "enabled": false}))
	got, err := r.FindByID(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, "new", got.Name)
	require.False(t, got.Enabled)
	deleted := &models.CloudAccount{Name: "deleted", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: models.StringArray{"us-east-1"}, Enabled: true}
	require.NoError(t, r.Create(ctx, deleted))
	require.NoError(t, r.SoftDelete(ctx, deleted.ID))
	count, err := r.CountByCloudKeyID(ctx, key.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	names, err := r.CloudKeyNames(ctx, []uint64{key.ID, otherKey.ID})
	require.NoError(t, err)
	require.Equal(t, "primary-key", names[key.ID])
	require.Equal(t, "other-key", names[otherKey.ID])
}

func TestRepo_ListAndDetailLatestSyncMetadata(t *testing.T) {
	r, gdb := setupRepo(t)
	ctx := context.Background()
	key := dbtest.SeedCloudKey(t, gdb, "sync-key")
	account := &models.CloudAccount{Name: "sync", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: models.StringArray{"us-east-1"}, Enabled: true}
	require.NoError(t, r.Create(ctx, account))
	old := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	latest := time.Now().UTC().Truncate(time.Microsecond)
	for _, run := range []models.AssetsSyncRun{
		{CloudAccountID: account.ID, Region: "us-east-1", ResourceType: "instance", StartedAt: old, Status: "failed", Trigger: "manual"},
		{CloudAccountID: account.ID, Region: "us-east-1", ResourceType: "network", StartedAt: latest, Status: "success", Trigger: "cron"},
	} {
		require.NoError(t, gdb.Create(&run).Error)
	}
	items, _, err := r.List(ctx, ListQuery{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "success", items[0].LastSyncStatus)
	require.WithinDuration(t, latest, *items[0].LastSyncAt, time.Microsecond)
	detail := toDetail(*account, "sync-key", items[0].LastSyncAt, items[0].LastSyncStatus)
	require.Equal(t, "success", detail.LastSyncStatus)
	require.Equal(t, []string{"us-east-1"}, detail.EnabledRegions)
}

func TestRepo_CascadeSoftDelete_TransactionRollback(t *testing.T) {
	r, gdb := setupRepo(t)
	ctx := context.Background()
	key := dbtest.SeedCloudKey(t, gdb, "rollback-key")
	account := &models.CloudAccount{Name: "rollback", Provider: "aws", CloudKeyID: key.ID, EnabledRegions: models.StringArray{"us-east-1"}, Enabled: true}
	require.NoError(t, r.Create(ctx, account))
	seedResourceSet(t, gdb, account.ID, "us-east-1", "rollback")
	wantErr := errors.New("force rollback")
	err := r.Transaction(ctx, func(tx *gorm.DB) error {
		count, err := r.CascadeSoftDeleteResources(ctx, tx, account.ID, nil)
		require.NoError(t, err)
		require.EqualValues(t, 4, count)
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)
	for _, model := range []any{&models.AWSInstance{}, &models.AWSVPC{}, &models.AWSSubnet{}, &models.AWSDatabase{}} {
		var count int64
		require.NoError(t, gdb.Model(model).Where("cloud_account_id = ?", account.ID).Count(&count).Error)
		require.EqualValues(t, 1, count)
	}
}

func TestRepo_TransactionalWritesRequireLiveRow(t *testing.T) {
	r, _ := setupRepo(t)
	ctx := context.Background()
	err := r.Transaction(ctx, func(tx *gorm.DB) error {
		_, err := r.FindByIDForUpdate(ctx, tx, 99999)
		bizErr, ok := apperr.AsBiz(err)
		require.True(t, ok)
		require.Equal(t, errs.CodeAssetsCloudAccountNotFound, bizErr.Code)
		updated, err := r.UpdateTx(ctx, tx, 99999, map[string]any{"name": "missing"})
		require.NoError(t, err)
		require.Zero(t, updated)
		deleted, err := r.SoftDeleteTx(ctx, tx, 99999)
		require.NoError(t, err)
		require.Zero(t, deleted)
		return nil
	})
	require.NoError(t, err)
}

func seedResourceSet(t *testing.T, gdb *gorm.DB, accountID uint64, region, suffix string) {
	t.Helper()
	now := time.Now()
	dbtest.SeedAWSInstance(t, gdb, accountID, region, "i-"+suffix)
	require.NoError(t, gdb.Create(&models.AWSVPC{CloudAccountID: accountID, Region: region, VPCID: "vpc-" + suffix, LastSeenAt: now}).Error)
	require.NoError(t, gdb.Create(&models.AWSSubnet{CloudAccountID: accountID, Region: region, SubnetID: "subnet-" + suffix, VPCID: "vpc-" + suffix, LastSeenAt: now}).Error)
	require.NoError(t, gdb.Create(&models.AWSDatabase{CloudAccountID: accountID, Region: region, DBInstanceID: "db-" + suffix, LastSeenAt: now}).Error)
}
