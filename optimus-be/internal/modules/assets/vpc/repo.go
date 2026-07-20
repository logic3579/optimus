package vpc

import (
	"context"
	"encoding/json"

	"optimus-be/internal/models"

	"gorm.io/gorm"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) List(ctx context.Context, filter ListFilter) ([]Summary, int64, error) {
	query := r.vpcQuery(ctx, filter)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []struct {
		models.AWSVPC
		CloudAccountName string `gorm:"column:cloud_account_name"`
	}
	if err := r.vpcQuery(ctx, filter).
		Select("aws_vpcs.*, cloud_accounts.name AS cloud_account_name").
		Joins("LEFT JOIN cloud_accounts ON cloud_accounts.id = aws_vpcs.cloud_account_id").
		Order("aws_vpcs.deleted_at IS NULL DESC, aws_vpcs.last_seen_at DESC, aws_vpcs.id DESC").
		Offset((filter.Page - 1) * filter.Size).
		Limit(filter.Size).
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	items := make([]Summary, len(rows))
	for i := range rows {
		items[i] = toSummary(rows[i].AWSVPC, rows[i].CloudAccountName)
	}
	return items, total, nil
}

func (r *Repo) vpcQuery(ctx context.Context, filter ListFilter) *gorm.DB {
	query := r.db.WithContext(ctx).Model(&models.AWSVPC{})
	if filter.IncludeDeleted {
		query = query.Unscoped()
	}
	if filter.AccountID != 0 {
		query = query.Where("aws_vpcs.cloud_account_id = ?", filter.AccountID)
	}
	if filter.Region != "" {
		query = query.Where("aws_vpcs.region = ?", filter.Region)
	}
	if filter.Q != "" {
		term := "%" + filter.Q + "%"
		query = query.Where("aws_vpcs.name ILIKE ? OR aws_vpcs.vpc_id ILIKE ?", term, term)
	}
	return query
}

func (r *Repo) FindByID(ctx context.Context, id uint64) (*models.AWSVPC, error) {
	var row models.AWSVPC
	if err := r.db.WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repo) ListSubnets(ctx context.Context, filter SubnetListFilter) ([]SubnetSummary, int64, error) {
	query := r.subnetQuery(ctx, filter)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []models.AWSSubnet
	if err := r.subnetQuery(ctx, filter).
		Order("aws_subnets.deleted_at IS NULL DESC, aws_subnets.last_seen_at DESC, aws_subnets.id DESC").
		Offset((filter.Page - 1) * filter.Size).
		Limit(filter.Size).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	items := make([]SubnetSummary, len(rows))
	for i := range rows {
		items[i] = toSubnetSummary(rows[i])
	}
	return items, total, nil
}

func (r *Repo) subnetQuery(ctx context.Context, filter SubnetListFilter) *gorm.DB {
	query := r.db.WithContext(ctx).Model(&models.AWSSubnet{})
	if filter.IncludeDeleted {
		query = query.Unscoped()
	}
	query = query.Where(
		"aws_subnets.cloud_account_id = ? AND aws_subnets.region = ? AND aws_subnets.vpc_id = ?",
		filter.CloudAccountID, filter.Region, filter.VPCID,
	)
	if filter.Q != "" {
		term := "%" + filter.Q + "%"
		query = query.Where("aws_subnets.name ILIKE ? OR aws_subnets.subnet_id ILIKE ?", term, term)
	}
	return query
}

func toSummary(row models.AWSVPC, accountName string) Summary {
	item := Summary{
		ID: row.ID, CloudAccountID: row.CloudAccountID, CloudAccountName: accountName,
		Region: row.Region, VPCID: row.VPCID, Name: row.Name, IsDefault: row.IsDefault,
		State: row.State, Tags: decodeTags(row.Tags), LastSeenAt: row.LastSeenAt,
		Deleted: row.DeletedAt.Valid,
	}
	if row.CIDRBlock != nil {
		item.CIDRBlock = *row.CIDRBlock
	}
	return item
}

func toSubnetSummary(row models.AWSSubnet) SubnetSummary {
	item := SubnetSummary{
		ID: row.ID, CloudAccountID: row.CloudAccountID, Region: row.Region,
		SubnetID: row.SubnetID, VPCID: row.VPCID, Name: row.Name,
		AvailabilityZone: row.AvailabilityZone, Tags: decodeTags(row.Tags),
		LastSeenAt: row.LastSeenAt, Deleted: row.DeletedAt.Valid,
	}
	if row.CIDRBlock != nil {
		item.CIDRBlock = *row.CIDRBlock
	}
	return item
}

func decodeTags(raw []byte) map[string]string {
	tags := make(map[string]string)
	if err := json.Unmarshal(raw, &tags); err != nil {
		return map[string]string{}
	}
	return tags
}
