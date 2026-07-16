package runs

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type Repo struct{ db *gorm.DB }

var ErrRunNotRunning = errors.New("sync run is missing or no longer running")

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) List(ctx context.Context, filter ListFilter) ([]Summary, int64, error) {
	query := r.db.WithContext(ctx).Table("assets_sync_runs")
	if filter.AccountID != 0 {
		query = query.Where("assets_sync_runs.cloud_account_id = ?", filter.AccountID)
	}
	if filter.ResourceType != "" {
		query = query.Where("assets_sync_runs.resource_type = ?", filter.ResourceType)
	}
	if filter.Status != "" {
		query = query.Where("assets_sync_runs.status = ?", filter.Status)
	}
	if filter.StartedAfter != nil {
		query = query.Where("assets_sync_runs.started_at >= ?", *filter.StartedAfter)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []struct {
		models.AssetsSyncRun
		CloudAccountName string `gorm:"column:cloud_account_name"`
	}
	err := query.
		Select("assets_sync_runs.*, cloud_accounts.name AS cloud_account_name").
		Joins("LEFT JOIN cloud_accounts ON cloud_accounts.id = assets_sync_runs.cloud_account_id").
		Order("assets_sync_runs.started_at DESC, assets_sync_runs.id DESC").
		Offset((filter.Page - 1) * filter.Size).
		Limit(filter.Size).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]Summary, len(rows))
	for i := range rows {
		row := rows[i]
		items[i] = Summary{
			ID: row.ID, CloudAccountID: row.CloudAccountID, CloudAccountName: row.CloudAccountName,
			Region: row.Region, ResourceType: row.ResourceType, StartedAt: row.StartedAt,
			FinishedAt: row.FinishedAt, Status: row.Status, ItemsSeen: row.ItemsSeen,
			ItemsSoftDeleted: row.ItemsSoftDeleted, Error: row.Error, ErrorCode: row.ErrorCode,
			Trigger: row.Trigger, TriggeredByUserID: row.TriggeredByUserID,
		}
	}
	return items, total, nil
}

type InsertRequest struct {
	CloudAccountID    uint64
	Region            string
	ResourceType      string
	Trigger           string
	TriggeredByUserID *uint64
}

func (r *Repo) Insert(ctx context.Context, req InsertRequest) (uint64, error) {
	row := &models.AssetsSyncRun{
		CloudAccountID:    req.CloudAccountID,
		Region:            req.Region,
		ResourceType:      req.ResourceType,
		StartedAt:         time.Now(),
		Status:            "running",
		Trigger:           req.Trigger,
		TriggeredByUserID: req.TriggeredByUserID,
	}
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

type FinishRequest struct {
	Status           string
	ItemsSeen        int32
	ItemsSoftDeleted int32
	Error            string
	ErrorCode        int32
}

func (r *Repo) Finish(ctx context.Context, id uint64, req FinishRequest) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&models.AssetsSyncRun{}).
		Where("id = ? AND status = ?", id, "running").
		Updates(map[string]any{
			"finished_at":       &now,
			"status":            req.Status,
			"items_seen":        req.ItemsSeen,
			"items_softdeleted": req.ItemsSoftDeleted,
			"error":             req.Error,
			"error_code":        req.ErrorCode,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrRunNotRunning
	}
	return nil
}

func (r *Repo) Prune(ctx context.Context, olderThanDays int) (int64, error) {
	if olderThanDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)
	result := r.db.WithContext(ctx).Where("started_at < ?", cutoff).Delete(&models.AssetsSyncRun{})
	return result.RowsAffected, result.Error
}
