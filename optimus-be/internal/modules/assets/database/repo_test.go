//go:build dbtest

package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/tests/dbtest"
)

func TestRepoListFiltersEveryPredicateAndMapsFields(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "database-list-key")
	first := dbtest.SeedCloudAccount(t, gdb, key.ID, "database-first", "us-east-1")
	second := dbtest.SeedCloudAccount(t, gdb, key.ID, "database-second", "eu-west-1")
	base := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	port, storage := int32(5432), int32(100)
	rows := []models.AWSDatabase{
		{CloudAccountID: first.ID, Region: "us-east-1", DBInstanceID: "orders-primary", Engine: "postgres", EngineVersion: "16.2", InstanceClass: "db.r6g.large", Status: "available", Endpoint: "orders.prod.internal", Port: &port, MultiAZ: true, PubliclyAccessible: false, StorageGB: &storage, Tags: datatypes.JSON([]byte(`{"env":"prod"}`)), LastSeenAt: base},
		{CloudAccountID: first.ID, Region: "us-west-2", DBInstanceID: "cache", Engine: "mysql", Status: "stopped", Endpoint: "cache.internal", Tags: datatypes.JSON([]byte(`{"env":"dev"}`)), LastSeenAt: base.Add(time.Minute)},
		{CloudAccountID: second.ID, Region: "eu-west-1", DBInstanceID: "orders-replica", Engine: "postgres", Status: "available", Endpoint: "replica.internal", Tags: datatypes.JSON([]byte(`{}`)), LastSeenAt: base.Add(2 * time.Minute)},
	}
	for i := range rows {
		require.NoError(t, gdb.Create(&rows[i]).Error)
	}
	require.NoError(t, gdb.Delete(&rows[1]).Error)
	require.NoError(t, gdb.Delete(first).Error)
	repo := NewRepo(gdb)

	assertIDs := func(query ListFilter, want ...uint64) {
		t.Helper()
		items, total, err := repo.List(context.Background(), query)
		require.NoError(t, err)
		require.EqualValues(t, len(want), total)
		ids := make([]uint64, len(items))
		for i := range items {
			ids[i] = items[i].ID
		}
		require.Equal(t, want, ids)
	}
	page := func() ListFilter { return ListFilter{Page: 1, Size: 20} }
	q := page()
	q.AccountID = first.ID
	assertIDs(q, rows[0].ID)
	q = page()
	q.Region = "eu-west-1"
	assertIDs(q, rows[2].ID)
	q = page()
	q.Engine = "postgres"
	assertIDs(q, rows[2].ID, rows[0].ID)
	q = page()
	q.Status = "available"
	assertIDs(q, rows[2].ID, rows[0].ID)
	q = page()
	q.Q = "primary"
	assertIDs(q, rows[0].ID)
	q = page()
	q.Q = "prod.internal"
	assertIDs(q, rows[0].ID)

	items, total, err := repo.List(context.Background(), ListFilter{AccountID: first.ID, Page: 1, Size: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, "database-first", items[0].CloudAccountName)
	require.Equal(t, "16.2", items[0].EngineVersion)
	require.Equal(t, "db.r6g.large", items[0].InstanceClass)
	require.Equal(t, "orders.prod.internal", items[0].Endpoint)
	require.Equal(t, &port, items[0].Port)
	require.Equal(t, &storage, items[0].StorageGB)
	require.True(t, items[0].MultiAZ)
	require.False(t, items[0].PubliclyAccessible)
	require.Equal(t, map[string]string{"env": "prod"}, items[0].Tags)
}

func TestRepoListIncludeDeletedPaginationAndStableOrder(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "database-page-key")
	account := dbtest.SeedCloudAccount(t, gdb, key.ID, "database-page", "us-east-1")
	seen := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	first := seedDatabase(t, gdb, account.ID, "db-a", seen)
	second := seedDatabase(t, gdb, account.ID, "db-b", seen)
	deleted := seedDatabase(t, gdb, account.ID, "db-c", seen.Add(time.Hour))
	require.NoError(t, gdb.Delete(deleted).Error)

	repo := NewRepo(gdb)
	items, total, err := repo.List(context.Background(), ListFilter{IncludeDeleted: true, Page: 1, Size: 2})
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.Equal(t, []uint64{second.ID, first.ID}, []uint64{items[0].ID, items[1].ID})
	items, total, err = repo.List(context.Background(), ListFilter{IncludeDeleted: true, Page: 2, Size: 2})
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.Len(t, items, 1)
	require.Equal(t, deleted.ID, items[0].ID)
	require.True(t, items[0].Deleted)
}

func seedDatabase(t *testing.T, gdb *gorm.DB, accountID uint64, id string, seen time.Time) *models.AWSDatabase {
	t.Helper()
	row := &models.AWSDatabase{CloudAccountID: accountID, Region: "us-east-1", DBInstanceID: id, Tags: datatypes.JSON([]byte(`{}`)), LastSeenAt: seen}
	require.NoError(t, gdb.Create(row).Error)
	return row
}
