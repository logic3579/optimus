//go:build dbtest

package sync

import (
	"context"
	"errors"
	"fmt"
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

func TestUpsertInstancesBatchesLargeAuthoritativeSweep(t *testing.T) {
	gdb, account := setupSweepDB(t)
	started := time.Now().UTC()
	items := make([]models.AWSInstance, 5000)
	for i := range items {
		items[i].InstanceID = fmt.Sprintf("i-%06d", i)
	}

	deleted, err := UpsertInstances(context.Background(), gdb, account.ID, "us-east-1", started, items)
	require.NoError(t, err)
	require.Zero(t, deleted)
	var alive int64
	require.NoError(t, gdb.Model(&models.AWSInstance{}).Where("cloud_account_id = ?", account.ID).Count(&alive).Error)
	require.EqualValues(t, len(items), alive)

	deleted, err = UpsertInstances(context.Background(), gdb, account.ID, "us-east-1", started.Add(time.Second), nil)
	require.NoError(t, err)
	require.EqualValues(t, len(items), deleted)
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

func TestUpsertVPCsAndSubnetsSoftDeletesAndRevivesTogether(t *testing.T) {
	gdb, account := setupSweepDB(t)
	first := time.Now().UTC()
	_, _, err := UpsertVPCsAndSubnets(context.Background(), gdb, account.ID, "us-east-1", first,
		[]models.AWSVPC{{VPCID: "vpc-a"}, {VPCID: "vpc-b"}},
		[]models.AWSSubnet{{SubnetID: "subnet-a", VPCID: "vpc-a"}, {SubnetID: "subnet-b", VPCID: "vpc-b"}})
	require.NoError(t, err)

	vpcDeleted, subnetDeleted, err := UpsertVPCsAndSubnets(context.Background(), gdb, account.ID, "us-east-1", first.Add(time.Second),
		[]models.AWSVPC{{VPCID: "vpc-a"}}, []models.AWSSubnet{{SubnetID: "subnet-a", VPCID: "vpc-a"}})
	require.NoError(t, err)
	require.EqualValues(t, 1, vpcDeleted)
	require.EqualValues(t, 1, subnetDeleted)

	vpcDeleted, subnetDeleted, err = UpsertVPCsAndSubnets(context.Background(), gdb, account.ID, "us-east-1", first.Add(2*time.Second),
		[]models.AWSVPC{{VPCID: "vpc-b"}}, []models.AWSSubnet{{SubnetID: "subnet-b", VPCID: "vpc-b"}})
	require.NoError(t, err)
	require.EqualValues(t, 1, vpcDeleted)
	require.EqualValues(t, 1, subnetDeleted)
	var alive int64
	require.NoError(t, gdb.Model(&models.AWSVPC{}).Where("vpc_id = ?", "vpc-b").Count(&alive).Error)
	require.EqualValues(t, 1, alive)
	require.NoError(t, gdb.Model(&models.AWSSubnet{}).Where("subnet_id = ?", "subnet-b").Count(&alive).Error)
	require.EqualValues(t, 1, alive)
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
	deleted, err = UpsertDatabases(context.Background(), gdb, account.ID, "us-east-1", first.Add(2*time.Second), []models.AWSDatabase{{DBInstanceID: "db-b"}})
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
	var alive int64
	require.NoError(t, gdb.Model(&models.AWSDatabase{}).Where("db_instance_id = ?", "db-b").Count(&alive).Error)
	require.EqualValues(t, 1, alive)
}

func TestNetworkPersistenceSkipsWhenAccountIsIneligible(t *testing.T) {
	gdb, account := setupSweepDB(t)
	require.NoError(t, gdb.Model(&models.CloudAccount{}).Where("id = ?", account.ID).Update("enabled", false).Error)

	vpcDeleted, subnetDeleted, err := UpsertVPCsAndSubnets(context.Background(), gdb, account.ID, "us-east-1", time.Now().UTC(),
		[]models.AWSVPC{{VPCID: "vpc-new"}}, []models.AWSSubnet{{SubnetID: "subnet-new", VPCID: "vpc-new"}})
	require.ErrorIs(t, err, ErrSweepIneligible)
	require.Zero(t, vpcDeleted)
	require.Zero(t, subnetDeleted)
	var count int64
	require.NoError(t, gdb.Unscoped().Model(&models.AWSVPC{}).Where("vpc_id = ?", "vpc-new").Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, gdb.Unscoped().Model(&models.AWSSubnet{}).Where("subnet_id = ?", "subnet-new").Count(&count).Error)
	require.Zero(t, count)
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
