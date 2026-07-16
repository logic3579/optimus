//go:build dbtest

package vpc

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/tests/dbtest"
)

func TestRepoListFiltersMapsOrdersAndIncludesDeleted(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "vpc-list-key")
	first := dbtest.SeedCloudAccount(t, gdb, key.ID, "vpc-first-account", "us-east-1")
	second := dbtest.SeedCloudAccount(t, gdb, key.ID, "vpc-second-account", "eu-west-1")
	base := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	cidrA, cidrB := "10.0.0.0/16", "10.1.0.0/16"
	rows := []models.AWSVPC{
		{CloudAccountID: first.ID, Region: "us-east-1", VPCID: "vpc-a", Name: "production", CIDRBlock: &cidrA, State: "available", Tags: datatypes.JSON(`{"env":"prod"}`), LastSeenAt: base},
		{CloudAccountID: first.ID, Region: "us-east-1", VPCID: "vpc-b", Name: "staging", CIDRBlock: &cidrB, LastSeenAt: base.Add(time.Minute)},
		{CloudAccountID: second.ID, Region: "eu-west-1", VPCID: "vpc-c", Name: "other", LastSeenAt: base.Add(2 * time.Minute)},
	}
	for i := range rows {
		require.NoError(t, gdb.Create(&rows[i]).Error)
	}
	require.NoError(t, gdb.Delete(&rows[1]).Error)
	require.NoError(t, gdb.Delete(first).Error)

	repo := NewRepo(gdb)
	items, total, err := repo.List(context.Background(), ListFilter{AccountID: first.ID, Region: "us-east-1", Q: "vpc-a", Page: 1, Size: 10})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, "vpc-first-account", items[0].CloudAccountName)
	require.Equal(t, cidrA, items[0].CIDRBlock)
	require.Equal(t, map[string]string{"env": "prod"}, items[0].Tags)
	require.False(t, items[0].Deleted)

	items, total, err = repo.List(context.Background(), ListFilter{AccountID: first.ID, Q: "stag", IncludeDeleted: true, Page: 1, Size: 10})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.True(t, items[0].Deleted)

	items, total, err = repo.List(context.Background(), ListFilter{AccountID: first.ID, IncludeDeleted: true, Page: 1, Size: 1})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Equal(t, rows[0].ID, items[0].ID, "alive rows must sort before newer deleted rows")

	items, total, err = repo.List(context.Background(), ListFilter{AccountID: second.ID, Region: "eu-west-1", Page: 1, Size: 10})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, rows[2].ID, items[0].ID)
}

func TestRepoListSubnetsScopesMapsFiltersAndFindExcludesDeletedVPC(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "subnet-list-key")
	account := dbtest.SeedCloudAccount(t, gdb, key.ID, "subnet-account", "us-east-1", "eu-west-1")
	base := time.Date(2026, 7, 16, 11, 0, 0, 0, time.UTC)
	vpc := &models.AWSVPC{CloudAccountID: account.ID, Region: "us-east-1", VPCID: "vpc-a", LastSeenAt: base}
	require.NoError(t, gdb.Create(vpc).Error)
	cidr := "10.0.1.0/24"
	rows := []models.AWSSubnet{
		{CloudAccountID: account.ID, Region: "us-east-1", VPCID: "vpc-a", SubnetID: "subnet-a", Name: "private-app", CIDRBlock: &cidr, AvailabilityZone: "us-east-1a", Tags: datatypes.JSON(`{"tier":"private"}`), LastSeenAt: base},
		{CloudAccountID: account.ID, Region: "us-east-1", VPCID: "vpc-a", SubnetID: "subnet-b", Name: "public", LastSeenAt: base.Add(time.Minute)},
		{CloudAccountID: account.ID, Region: "us-east-1", VPCID: "vpc-other", SubnetID: "subnet-c", Name: "private-wrong-vpc", LastSeenAt: base.Add(2 * time.Minute)},
		{CloudAccountID: account.ID, Region: "eu-west-1", VPCID: "vpc-a", SubnetID: "subnet-d", Name: "private-wrong-region", LastSeenAt: base.Add(3 * time.Minute)},
	}
	for i := range rows {
		require.NoError(t, gdb.Create(&rows[i]).Error)
	}
	require.NoError(t, gdb.Delete(&rows[1]).Error)

	repo := NewRepo(gdb)
	found, err := repo.FindByID(context.Background(), vpc.ID)
	require.NoError(t, err)
	require.Equal(t, vpc.VPCID, found.VPCID)

	items, total, err := repo.ListSubnets(context.Background(), SubnetListFilter{
		CloudAccountID: account.ID, Region: "us-east-1", VPCID: "vpc-a", Q: "private", Page: 1, Size: 10,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, "subnet-a", items[0].SubnetID)
	require.Equal(t, cidr, items[0].CIDRBlock)
	require.Equal(t, map[string]string{"tier": "private"}, items[0].Tags)

	items, total, err = repo.ListSubnets(context.Background(), SubnetListFilter{
		CloudAccountID: account.ID, Region: "us-east-1", VPCID: "vpc-a", IncludeDeleted: true, Page: 1, Size: 10,
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, items, 2)
	require.False(t, items[0].Deleted)
	require.True(t, items[1].Deleted)

	require.NoError(t, gdb.Delete(vpc).Error)
	_, err = repo.FindByID(context.Background(), vpc.ID)
	require.Error(t, err)
}
