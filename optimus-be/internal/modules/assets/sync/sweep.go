// Package sync implements authoritative AWS asset discovery sweeps.
package sync

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"optimus-be/internal/models"
)

// ErrSweepIneligible means the account was deleted, disabled, or no longer
// enables the region between fetch and persistence. Callers must mark the run
// skipped; no resource rows are changed.
var ErrSweepIneligible = errors.New("asset sweep target is no longer eligible")

const sweepBatchSize = 500

func UpsertInstances(ctx context.Context, db *gorm.DB, accountID uint64, region string, sweepStart time.Time, items []models.AWSInstance) (int64, error) {
	var softDeleted int64
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockEligibleAccount(tx, accountID, region); err != nil {
			return err
		}
		for i := range items {
			items[i].ID = 0
			items[i].CloudAccountID = accountID
			items[i].Region = region
			items[i].LastSeenAt = sweepStart
			items[i].DeletedAt = gorm.DeletedAt{}
		}
		if len(items) > 0 {
			if err := tx.Clauses(instanceConflict()).CreateInBatches(&items, sweepBatchSize).Error; err != nil {
				return err
			}
		}
		result := tx.Where("cloud_account_id = ? AND region = ? AND last_seen_at < ?", accountID, region, sweepStart).
			Delete(&models.AWSInstance{})
		softDeleted = result.RowsAffected
		return result.Error
	})
	return softDeleted, err
}

func UpsertVPCsAndSubnets(ctx context.Context, db *gorm.DB, accountID uint64, region string, sweepStart time.Time, vpcs []models.AWSVPC, subnets []models.AWSSubnet) (int64, int64, error) {
	var vpcSoftDeleted, subnetSoftDeleted int64
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockEligibleAccount(tx, accountID, region); err != nil {
			return err
		}
		for i := range vpcs {
			vpcs[i].ID = 0
			vpcs[i].CloudAccountID = accountID
			vpcs[i].Region = region
			vpcs[i].LastSeenAt = sweepStart
			vpcs[i].DeletedAt = gorm.DeletedAt{}
		}
		for i := range subnets {
			subnets[i].ID = 0
			subnets[i].CloudAccountID = accountID
			subnets[i].Region = region
			subnets[i].LastSeenAt = sweepStart
			subnets[i].DeletedAt = gorm.DeletedAt{}
		}
		if len(vpcs) > 0 {
			if err := tx.Clauses(vpcConflict()).CreateInBatches(&vpcs, sweepBatchSize).Error; err != nil {
				return err
			}
		}
		if len(subnets) > 0 {
			if err := tx.Clauses(subnetConflict()).CreateInBatches(&subnets, sweepBatchSize).Error; err != nil {
				return err
			}
		}
		vpcResult := tx.Where("cloud_account_id = ? AND region = ? AND last_seen_at < ?", accountID, region, sweepStart).
			Delete(&models.AWSVPC{})
		if vpcResult.Error != nil {
			return vpcResult.Error
		}
		vpcSoftDeleted = vpcResult.RowsAffected
		subnetResult := tx.Where("cloud_account_id = ? AND region = ? AND last_seen_at < ?", accountID, region, sweepStart).
			Delete(&models.AWSSubnet{})
		subnetSoftDeleted = subnetResult.RowsAffected
		return subnetResult.Error
	})
	return vpcSoftDeleted, subnetSoftDeleted, err
}

func UpsertDatabases(ctx context.Context, db *gorm.DB, accountID uint64, region string, sweepStart time.Time, items []models.AWSDatabase) (int64, error) {
	var softDeleted int64
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockEligibleAccount(tx, accountID, region); err != nil {
			return err
		}
		for i := range items {
			items[i].ID = 0
			items[i].CloudAccountID = accountID
			items[i].Region = region
			items[i].LastSeenAt = sweepStart
			items[i].DeletedAt = gorm.DeletedAt{}
		}
		if len(items) > 0 {
			if err := tx.Clauses(databaseConflict()).CreateInBatches(&items, sweepBatchSize).Error; err != nil {
				return err
			}
		}
		result := tx.Where("cloud_account_id = ? AND region = ? AND last_seen_at < ?", accountID, region, sweepStart).
			Delete(&models.AWSDatabase{})
		softDeleted = result.RowsAffected
		return result.Error
	})
	return softDeleted, err
}

func lockEligibleAccount(tx *gorm.DB, accountID uint64, region string) error {
	var account models.CloudAccount
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, accountID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrSweepIneligible
	}
	if err != nil {
		return err
	}
	if !account.Enabled {
		return ErrSweepIneligible
	}
	for _, enabledRegion := range account.EnabledRegions {
		if enabledRegion == region {
			return nil
		}
	}
	return ErrSweepIneligible
}

func instanceConflict() clause.OnConflict {
	return clause.OnConflict{
		Columns:   []clause.Column{{Name: "cloud_account_id"}, {Name: "region"}, {Name: "instance_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "instance_type", "state", "private_ip", "public_ip", "vpc_id", "subnet_id", "availability_zone", "launch_time", "tags", "last_seen_at", "updated_at", "deleted_at"}),
	}
}

func vpcConflict() clause.OnConflict {
	return clause.OnConflict{
		Columns:   []clause.Column{{Name: "cloud_account_id"}, {Name: "region"}, {Name: "vpc_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "cidr_block", "is_default", "state", "tags", "last_seen_at", "updated_at", "deleted_at"}),
	}
}

func subnetConflict() clause.OnConflict {
	return clause.OnConflict{
		Columns:   []clause.Column{{Name: "cloud_account_id"}, {Name: "region"}, {Name: "subnet_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"vpc_id", "cidr_block", "availability_zone", "name", "tags", "last_seen_at", "updated_at", "deleted_at"}),
	}
}

func databaseConflict() clause.OnConflict {
	return clause.OnConflict{
		Columns:   []clause.Column{{Name: "cloud_account_id"}, {Name: "region"}, {Name: "db_instance_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"engine", "engine_version", "instance_class", "status", "endpoint", "port", "multi_az", "publicly_accessible", "storage_gb", "tags", "last_seen_at", "updated_at", "deleted_at"}),
	}
}
