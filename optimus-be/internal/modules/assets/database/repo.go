package database

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) List(ctx context.Context, filter ListFilter) ([]Summary, int64, error) {
	query := r.db.WithContext(ctx).Table("aws_databases")
	if !filter.IncludeDeleted {
		query = query.Where("aws_databases.deleted_at IS NULL")
	}
	if filter.AccountID != 0 {
		query = query.Where("aws_databases.cloud_account_id = ?", filter.AccountID)
	}
	if filter.Region != "" {
		query = query.Where("aws_databases.region = ?", filter.Region)
	}
	if filter.Engine != "" {
		query = query.Where("aws_databases.engine = ?", filter.Engine)
	}
	if filter.Status != "" {
		query = query.Where("aws_databases.status = ?", filter.Status)
	}
	if filter.Q != "" {
		like := "%" + filter.Q + "%"
		query = query.Where("(aws_databases.db_instance_id ILIKE ? OR aws_databases.endpoint ILIKE ?)", like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []struct {
		models.AWSDatabase
		CloudAccountName string `gorm:"column:cloud_account_name"`
	}
	err := query.
		Select("aws_databases.*, cloud_accounts.name AS cloud_account_name").
		Joins("LEFT JOIN cloud_accounts ON cloud_accounts.id = aws_databases.cloud_account_id").
		Order("aws_databases.deleted_at IS NULL DESC, aws_databases.last_seen_at DESC, aws_databases.id DESC").
		Offset((filter.Page - 1) * filter.Size).
		Limit(filter.Size).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]Summary, len(rows))
	for i := range rows {
		items[i] = toSummary(rows[i].AWSDatabase, rows[i].CloudAccountName)
	}
	return items, total, nil
}

func toSummary(row models.AWSDatabase, accountName string) Summary {
	tags := map[string]string{}
	_ = json.Unmarshal(row.Tags, &tags)
	return Summary{
		ID: row.ID, CloudAccountID: row.CloudAccountID, CloudAccountName: accountName,
		Region: row.Region, DBInstanceID: row.DBInstanceID, Engine: row.Engine,
		EngineVersion: row.EngineVersion, InstanceClass: row.InstanceClass, Status: row.Status,
		Endpoint: row.Endpoint, Port: row.Port, MultiAZ: row.MultiAZ,
		PubliclyAccessible: row.PubliclyAccessible, StorageGB: row.StorageGB, Tags: tags,
		LastSeenAt: row.LastSeenAt, Deleted: row.DeletedAt.Valid,
	}
}
