//go:build dbtest

package assets

import (
	"context"
	"net"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"optimus-be/internal/infra/db"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/tests/dbtest"

	"github.com/stretchr/testify/require"
)

func TestConsumerAllLookupsScopeMapOrderAndExcludeDeleted(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "assets-consumer-key")
	first := dbtest.SeedCloudAccount(t, gdb, key.ID, "first-account", "us-east-1", "eu-west-1")
	second := dbtest.SeedCloudAccount(t, gdb, key.ID, "second-account", "us-east-1")

	base := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	privateIPv4 := net.ParseIP("10.0.0.5")
	publicIPv6 := net.ParseIP("2001:db8::5")
	sharedPrivateIP := net.ParseIP("10.0.0.9")
	rows := []models.AWSInstance{
		{
			CloudAccountID: first.ID, Region: "us-east-1", InstanceID: "i-main",
			Name: "main", InstanceType: "m7g.large", State: "running",
			PrivateIP: &privateIPv4, PublicIP: &publicIPv6, VPCID: "vpc-main", SubnetID: "subnet-a",
			LastSeenAt: base,
		},
		{
			CloudAccountID: first.ID, Region: "us-east-1", InstanceID: "i-tie-low",
			Name: "tie-low", VPCID: "vpc-main", LastSeenAt: base.Add(time.Minute),
		},
		{
			CloudAccountID: first.ID, Region: "us-east-1", InstanceID: "i-tie-high",
			Name: "tie-high", VPCID: "vpc-main", LastSeenAt: base.Add(time.Minute),
		},
		{
			CloudAccountID: first.ID, Region: "eu-west-1", InstanceID: "i-other-region",
			Name: "other-region", VPCID: "vpc-main", LastSeenAt: base.Add(2 * time.Minute),
		},
		{
			CloudAccountID: second.ID, Region: "us-east-1", InstanceID: "i-other-account",
			Name: "other-account", VPCID: "vpc-main", LastSeenAt: base.Add(3 * time.Minute),
		},
		{
			CloudAccountID: first.ID, Region: "us-east-1", InstanceID: "i-shared-old",
			Name: "shared-old", PrivateIP: &sharedPrivateIP, VPCID: "vpc-shared", LastSeenAt: base,
		},
		{
			CloudAccountID: second.ID, Region: "us-east-1", InstanceID: "i-shared-new",
			Name: "shared-new", PrivateIP: &sharedPrivateIP, VPCID: "vpc-shared", LastSeenAt: base.Add(time.Minute),
		},
	}
	for i := range rows {
		require.NoError(t, gdb.Create(&rows[i]).Error)
	}
	deleted := models.AWSInstance{
		CloudAccountID: first.ID, Region: "us-east-1", InstanceID: "i-deleted",
		PrivateIP: &privateIPv4, VPCID: "vpc-main", LastSeenAt: base.Add(10 * time.Minute),
	}
	require.NoError(t, gdb.Create(&deleted).Error)
	require.NoError(t, gdb.Delete(&deleted).Error)
	// The account name remains useful historical metadata after account soft-delete.
	require.NoError(t, gdb.Delete(first).Error)

	c := NewConsumer(gdb)
	ctx := context.Background()

	got, err := c.LookupInstanceByPrivateIP(ctx, netip.MustParseAddr("::ffff:10.0.0.5"))
	require.NoError(t, err)
	require.Equal(t, "i-main", got.InstanceID, "soft-deleted assets must not win lookup ordering")
	require.Equal(t, "first-account", got.AccountName)
	require.Equal(t, "10.0.0.5", got.PrivateIP.String())
	require.Equal(t, "2001:db8::5", got.PublicIP.String())

	got, err = c.LookupInstanceByID(ctx, int64(first.ID), "us-east-1", "i-main")
	require.NoError(t, err)
	require.Equal(t, "main", got.Name)
	require.Equal(t, "m7g.large", got.InstanceType)
	require.Equal(t, "running", got.State)
	require.Equal(t, "subnet-a", got.SubnetID)
	_, err = c.LookupInstanceByID(ctx, int64(second.ID), "us-east-1", "i-main")
	require.ErrorIs(t, err, ErrAssetsInstanceNotFound, "ID lookup must stay inside account tuple")
	_, err = c.LookupInstanceByID(ctx, int64(first.ID), "eu-west-1", "i-main")
	require.ErrorIs(t, err, ErrAssetsInstanceNotFound, "ID lookup must stay inside region tuple")

	got, err = c.LookupInstanceByPrivateIP(ctx, netip.MustParseAddr("10.0.0.9"))
	require.NoError(t, err)
	require.Equal(t, "i-shared-new", got.InstanceID, "ambiguous private IP uses newest snapshot")

	list, err := c.ListInstancesByVPC(ctx, int64(first.ID), "us-east-1", "vpc-main")
	require.NoError(t, err)
	require.Equal(t, []string{"i-tie-high", "i-tie-low", "i-main"}, instanceIDs(list))
	require.False(t, list[0].PrivateIP.IsValid(), "nullable IP remains netip zero value")
	require.False(t, list[0].PublicIP.IsValid())
	for _, item := range list {
		require.Equal(t, first.ID, uint64(item.AccountID), "list must stay inside account tuple")
		require.Equal(t, "us-east-1", item.Region, "list must stay inside region tuple")
	}

	empty, err := c.ListInstancesByVPC(ctx, int64(first.ID), "us-east-1", "vpc-empty")
	require.NoError(t, err)
	require.NotNil(t, empty)
	require.Empty(t, empty)

	_, err = c.LookupInstanceByID(ctx, int64(first.ID), "us-east-1", "i-deleted")
	require.ErrorIs(t, err, ErrAssetsInstanceNotFound)
	_, err = c.LookupInstanceByPrivateIP(ctx, netip.MustParseAddr("192.0.2.99"))
	require.ErrorIs(t, err, ErrAssetsInstanceNotFound)
}

func TestConsumerDatabaseErrorsPreserveDiagnosticCause(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	c := NewConsumer(gdb)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.LookupInstanceByPrivateIP(canceled, netip.MustParseAddr("10.0.0.1"))
	require.ErrorIs(t, err, context.Canceled)
	var biz *apperr.BizError
	require.ErrorAs(t, err, &biz)
	require.Equal(t, apperr.CodeDBError, biz.Code)

	_, err = c.LookupInstanceByID(canceled, 1, "us-east-1", "i-a")
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorAs(t, err, &biz)
	require.Equal(t, apperr.CodeDBError, biz.Code)

	_, err = c.ListInstancesByVPC(canceled, 1, "us-east-1", "vpc-a")
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorAs(t, err, &biz)
	require.Equal(t, apperr.CodeDBError, biz.Code)
}

func instanceIDs(items []Instance) []string {
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].InstanceID
	}
	return ids
}
