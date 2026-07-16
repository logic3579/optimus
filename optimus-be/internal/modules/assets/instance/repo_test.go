//go:build dbtest

package instance

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/tests/dbtest"
)

func TestRepoListFiltersSearchesMapsOrdersAndPaginates(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "instance-list-key")
	firstAccount := dbtest.SeedCloudAccount(t, gdb, key.ID, "first-account", "us-east-1")
	secondAccount := dbtest.SeedCloudAccount(t, gdb, key.ID, "second-account", "eu-west-1")

	base := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	privateIP := net.ParseIP("10.0.0.5")
	publicIP := net.ParseIP("203.0.113.8")
	launchTime := base.Add(-24 * time.Hour)
	rows := []models.AWSInstance{
		{
			CloudAccountID: firstAccount.ID, Region: "us-east-1", InstanceID: "i-web-a",
			Name: "checkout-web", InstanceType: "t3.small", State: "running",
			PrivateIP: &privateIP, PublicIP: &publicIP, VPCID: "vpc-main", SubnetID: "subnet-a",
			AvailabilityZone: "us-east-1a", LaunchTime: &launchTime,
			Tags: datatypes.JSON(`{"role":"payments","team":"platform"}`), LastSeenAt: base.Add(time.Minute),
		},
		{
			CloudAccountID: firstAccount.ID, Region: "us-west-2", InstanceID: "i-worker-b",
			Name: "batch-worker", State: "stopped", VPCID: "vpc-other",
			Tags: datatypes.JSON(`{"role":"worker"}`), LastSeenAt: base.Add(time.Minute),
		},
		{
			CloudAccountID: secondAccount.ID, Region: "eu-west-1", InstanceID: "i-db-c",
			Name: "db-helper", State: "running", VPCID: "vpc-eu",
			Tags: datatypes.JSON(`{"role":"database"}`), LastSeenAt: base.Add(2 * time.Minute),
		},
	}
	for i := range rows {
		require.NoError(t, gdb.Create(&rows[i]).Error)
	}
	deleted := models.AWSInstance{
		CloudAccountID: firstAccount.ID, Region: "us-east-1", InstanceID: "i-old",
		Name: "retired", State: "terminated", VPCID: "vpc-main",
		Tags: datatypes.JSON(`{"role":"legacy"}`), LastSeenAt: base.Add(3 * time.Minute),
	}
	require.NoError(t, gdb.Create(&deleted).Error)
	require.NoError(t, gdb.Delete(&deleted).Error)
	// The account join intentionally remains readable after the parent account is soft-deleted.
	require.NoError(t, gdb.Delete(firstAccount).Error)

	repo := NewRepo(gdb)
	filterCases := []struct {
		name   string
		filter ListFilter
		wantID uint64
	}{
		{name: "account", filter: ListFilter{AccountID: secondAccount.ID}, wantID: rows[2].ID},
		{name: "region", filter: ListFilter{Region: "us-west-2"}, wantID: rows[1].ID},
		{name: "state", filter: ListFilter{State: "stopped"}, wantID: rows[1].ID},
		{name: "vpc", filter: ListFilter{VPCID: "vpc-eu"}, wantID: rows[2].ID},
		{name: "q name", filter: ListFilter{Q: "CHECKOUT"}, wantID: rows[0].ID},
		{name: "q instance id", filter: ListFilter{Q: "WEB-A"}, wantID: rows[0].ID},
		{name: "q private ip", filter: ListFilter{Q: "0.0.5"}, wantID: rows[0].ID},
		{name: "q tag value", filter: ListFilter{Q: "PAYMENT"}, wantID: rows[0].ID},
	}
	for _, test := range filterCases {
		t.Run(test.name, func(t *testing.T) {
			filter := test.filter
			filter.Page, filter.Size = 1, 20
			items, total, err := repo.List(context.Background(), filter)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, items, 1)
			require.Equal(t, test.wantID, items[0].ID)
		})
	}

	items, total, err := repo.List(context.Background(), ListFilter{AccountID: firstAccount.ID, IncludeDeleted: true, Page: 1, Size: 2})
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.Len(t, items, 2)
	// Alive rows sort before deleted rows even when the deleted row was seen later;
	// equal last_seen_at values use descending ID as a stable tiebreaker.
	require.Equal(t, []uint64{rows[1].ID, rows[0].ID}, []uint64{items[0].ID, items[1].ID})
	require.Equal(t, "first-account", items[0].CloudAccountName)

	items, total, err = repo.List(context.Background(), ListFilter{AccountID: firstAccount.ID, IncludeDeleted: true, Page: 2, Size: 2, Offset: 2})
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.Len(t, items, 1)
	require.Equal(t, deleted.ID, items[0].ID)
	require.True(t, items[0].Deleted)

	items, total, err = repo.List(context.Background(), ListFilter{Q: "role", IncludeDeleted: true, Page: 1, Size: 20})
	require.NoError(t, err)
	require.Zero(t, total, "q must not match tag keys")
	require.Empty(t, items)
}

func TestRepoListMapsIPsTagsAndNullableFields(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "instance-map-key")
	account := dbtest.SeedCloudAccount(t, gdb, key.ID, "map-account", "us-east-1")
	privateIP := net.ParseIP("10.1.2.3")
	publicIP := net.ParseIP("2001:db8::1")
	launchTime := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	row := models.AWSInstance{
		CloudAccountID: account.ID, Region: "us-east-1", InstanceID: "i-map",
		Name: "mapped", InstanceType: "m7g.large", State: "running",
		PrivateIP: &privateIP, PublicIP: &publicIP, VPCID: "vpc-map", SubnetID: "subnet-map",
		AvailabilityZone: "us-east-1b", LaunchTime: &launchTime,
		Tags: datatypes.JSON(`{"Name":"mapped","env":"prod"}`), LastSeenAt: launchTime.Add(time.Hour),
	}
	require.NoError(t, gdb.Create(&row).Error)

	items, total, err := NewRepo(gdb).List(context.Background(), ListFilter{Page: 1, Size: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	got := items[0]
	require.Equal(t, "10.1.2.3", got.PrivateIP)
	require.Equal(t, "2001:db8::1", got.PublicIP)
	require.Equal(t, map[string]string{"Name": "mapped", "env": "prod"}, got.Tags)
	require.NotNil(t, got.LaunchTime)
	require.True(t, got.LaunchTime.Equal(launchTime))
	require.False(t, got.Deleted)
}
