package runs

import (
	"context"
	"time"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

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
	return r.db.WithContext(ctx).Model(&models.AssetsSyncRun{}).Where("id = ?", id).
		Updates(map[string]any{
			"finished_at":       &now,
			"status":            req.Status,
			"items_seen":        req.ItemsSeen,
			"items_softdeleted": req.ItemsSoftDeleted,
			"error":             req.Error,
			"error_code":        req.ErrorCode,
		}).Error
}

func (r *Repo) Prune(ctx context.Context, olderThanDays int) (int64, error) {
	if olderThanDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)
	result := r.db.WithContext(ctx).Where("started_at < ?", cutoff).Delete(&models.AssetsSyncRun{})
	return result.RowsAffected, result.Error
}
