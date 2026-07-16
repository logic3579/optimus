package instance

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) List(ctx context.Context, filter ListFilter) ([]Summary, int64, error) {
	query := r.db.WithContext(ctx).Table("aws_instances")
	if !filter.IncludeDeleted {
		query = query.Where("aws_instances.deleted_at IS NULL")
	}
	if filter.AccountID != 0 {
		query = query.Where("aws_instances.cloud_account_id = ?", filter.AccountID)
	}
	if filter.Region != "" {
		query = query.Where("aws_instances.region = ?", filter.Region)
	}
	if filter.State != "" {
		query = query.Where("aws_instances.state = ?", filter.State)
	}
	if filter.VPCID != "" {
		query = query.Where("aws_instances.vpc_id = ?", filter.VPCID)
	}
	if filter.Q != "" {
		like := "%" + filter.Q + "%"
		query = query.Where(`(
			aws_instances.name ILIKE ? OR
			aws_instances.instance_id ILIKE ? OR
			host(aws_instances.private_ip) ILIKE ? OR
			EXISTS (
				SELECT 1
				FROM jsonb_each_text(aws_instances.tags) AS tag(key, value)
				WHERE tag.value ILIKE ?
			)
		)`, like, like, like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []struct {
		models.AWSInstance
		CloudAccountName string         `gorm:"column:cloud_account_name"`
		PrivateIPText    sql.NullString `gorm:"column:private_ip_text"`
		PublicIPText     sql.NullString `gorm:"column:public_ip_text"`
	}
	err := query.
		Select(`
			aws_instances.id, aws_instances.cloud_account_id, aws_instances.region,
			aws_instances.instance_id, aws_instances.name, aws_instances.instance_type,
			aws_instances.state, aws_instances.vpc_id, aws_instances.subnet_id,
			aws_instances.availability_zone, aws_instances.launch_time, aws_instances.tags,
			aws_instances.last_seen_at, aws_instances.created_at, aws_instances.updated_at,
			aws_instances.deleted_at,
			host(aws_instances.private_ip) AS private_ip_text,
			host(aws_instances.public_ip) AS public_ip_text,
			cloud_accounts.name AS cloud_account_name`).
		Joins("LEFT JOIN cloud_accounts ON cloud_accounts.id = aws_instances.cloud_account_id").
		Order("aws_instances.deleted_at IS NULL DESC, aws_instances.last_seen_at DESC, aws_instances.id DESC").
		Offset(filter.Offset).
		Limit(filter.Size).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]Summary, len(rows))
	for i := range rows {
		items[i] = toSummary(rows[i].AWSInstance, rows[i].CloudAccountName)
		if rows[i].PrivateIPText.Valid {
			items[i].PrivateIP = normalizedIP(rows[i].PrivateIPText.String)
		}
		if rows[i].PublicIPText.Valid {
			items[i].PublicIP = normalizedIP(rows[i].PublicIPText.String)
		}
	}
	return items, total, nil
}

func normalizedIP(value string) string {
	if parsed := net.ParseIP(value); parsed != nil {
		return parsed.String()
	}
	return value
}

func toSummary(model models.AWSInstance, accountName string) Summary {
	result := Summary{
		ID: model.ID, CloudAccountID: model.CloudAccountID, CloudAccountName: accountName,
		Region: model.Region, InstanceID: model.InstanceID, Name: model.Name,
		InstanceType: model.InstanceType, State: model.State, VPCID: model.VPCID,
		SubnetID: model.SubnetID, AvailabilityZone: model.AvailabilityZone,
		LaunchTime: model.LaunchTime, Tags: map[string]string{}, LastSeenAt: model.LastSeenAt,
		Deleted: model.DeletedAt.Valid,
	}
	if model.PrivateIP != nil {
		result.PrivateIP = model.PrivateIP.String()
	}
	if model.PublicIP != nil {
		result.PublicIP = model.PublicIP.String()
	}
	_ = json.Unmarshal(model.Tags, &result.Tags)
	if result.Tags == nil {
		result.Tags = map[string]string{}
	}
	return result
}
