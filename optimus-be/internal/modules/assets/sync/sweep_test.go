//go:build dbtest

package sync

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/tests/dbtest"
)

func setupSweepDB(t *testing.T) (*gorm.DB, *models.CloudAccount) {
	t.Helper()
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "sweep-key-"+t.Name())
	return gdb, dbtest.SeedCloudAccount(t, gdb, key.ID, "sweep-account-"+t.Name(), "us-east-1")
}

func TestUpsertInstancesSoftDeletesMissingAndRevivesReturnedRows(t *testing.T) {
	gdb, account := setupSweepDB(t)
	ctx := context.Background()
	first := time.Now().UTC()

	softDeleted, err := UpsertInstances(ctx, gdb, account.ID, "us-east-1", first, []models.AWSInstance{
		{InstanceID: "i-a", State: "running"},
		{InstanceID: "i-b", State: "running"},
	})
	require.NoError(t, err)
	require.Zero(t, softDeleted)

	second := first.Add(time.Second)
	softDeleted, err = UpsertInstances(ctx, gdb, account.ID, "us-east-1", second, []models.AWSInstance{{InstanceID: "i-a", State: "stopped"}})
	require.NoError(t, err)
	require.EqualValues(t, 1, softDeleted)

	third := second.Add(time.Second)
	softDeleted, err = UpsertInstances(ctx, gdb, account.ID, "us-east-1", third, []models.AWSInstance{{InstanceID: "i-b", State: "running"}})
	require.NoError(t, err)
	require.EqualValues(t, 1, softDeleted)
	var revived models.AWSInstance
	require.NoError(t, gdb.Where("cloud_account_id = ? AND region = ? AND instance_id = ?", account.ID, "us-east-1", "i-b").First(&revived).Error)
	require.Equal(t, "running", revived.State)
}

func TestUpsertVPCsAndSubnetsIsOneAtomicSweep(t *testing.T) {
	gdb, account := setupSweepDB(t)
	started := time.Now().UTC()
	vpcs := []models.AWSVPC{{VPCID: "vpc-a"}}
	subnets := []models.AWSSubnet{{SubnetID: "subnet-a", VPCID: "vpc-a"}}

	vpcDeleted, subnetDeleted, err := UpsertVPCsAndSubnets(context.Background(), gdb, account.ID, "us-east-1", started, vpcs, subnets)
	require.NoError(t, err)
	require.Zero(t, vpcDeleted)
	require.Zero(t, subnetDeleted)

	// A bad subnet must roll back the VPC upsert from the same transaction.
	_, _, err = UpsertVPCsAndSubnets(context.Background(), gdb, account.ID, "us-east-1", started.Add(time.Second),
		[]models.AWSVPC{{VPCID: "vpc-b"}}, []models.AWSSubnet{{SubnetID: "subnet-b", VPCID: "vpc-b"}, {SubnetID: "subnet-b", VPCID: "vpc-b"}})
	require.Error(t, err)
	var count int64
	require.NoError(t, gdb.Model(&models.AWSVPC{}).Where("vpc_id = ?", "vpc-b").Count(&count).Error)
	require.Zero(t, count)
}

func TestUpsertDatabasesSoftDeletesMissing(t *testing.T) {
	gdb, account := setupSweepDB(t)
	first := time.Now().UTC()
	_, err := UpsertDatabases(context.Background(), gdb, account.ID, "us-east-1", first, []models.AWSDatabase{
		{DBInstanceID: "db-a"}, {DBInstanceID: "db-b"},
	})
	require.NoError(t, err)
	deleted, err := UpsertDatabases(context.Background(), gdb, account.ID, "us-east-1", first.Add(time.Second), []models.AWSDatabase{{DBInstanceID: "db-a"}})
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
}

func TestAuthoritativePersistenceSkipsWhenAccountBecomesIneligible(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*gorm.DB, uint64) error
	}{
		{name: "disabled", mutate: func(db *gorm.DB, id uint64) error {
			return db.Model(&models.CloudAccount{}).Where("id = ?", id).Update("enabled", false).Error
		}},
		{name: "deleted", mutate: func(db *gorm.DB, id uint64) error { return db.Delete(&models.CloudAccount{}, id).Error }},
		{name: "region removed", mutate: func(db *gorm.DB, id uint64) error {
			return db.Model(&models.CloudAccount{}).Where("id = ?", id).Update("enabled_regions", models.StringArray{"eu-west-1"}).Error
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gdb, account := setupSweepDB(t)
			old := dbtest.SeedAWSInstance(t, gdb, account.ID, "us-east-1", "i-old")
			require.NoError(t, tt.mutate(gdb, account.ID))

			deleted, err := UpsertInstances(context.Background(), gdb, account.ID, "us-east-1", time.Now().Add(time.Second), []models.AWSInstance{{InstanceID: "i-new"}})
			require.ErrorIs(t, err, ErrSweepIneligible)
			require.Zero(t, deleted)
			var oldAfter models.AWSInstance
			require.NoError(t, gdb.First(&oldAfter, old.ID).Error)
			var newCount int64
			require.NoError(t, gdb.Unscoped().Model(&models.AWSInstance{}).Where("instance_id = ?", "i-new").Count(&newCount).Error)
			require.Zero(t, newCount)
		})
	}
}

func TestSweepIneligibleIsRecognizable(t *testing.T) {
	require.True(t, errors.Is(ErrSweepIneligible, ErrSweepIneligible))
}
