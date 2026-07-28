package event

import (
	"context"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) RunExists(ctx context.Context, runID uint64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.DeliveryRun{}).Where("id = ?", runID).Limit(1).Count(&count).Error
	return count == 1, err
}

// ListAfter returns only committed rows because it executes outside the
// transaction that appends state transitions and their events.
func (r *Repo) ListAfter(ctx context.Context, runID, cursor uint64, limit int) ([]models.DeliveryRunEvent, error) {
	var rows []models.DeliveryRunEvent
	err := r.db.WithContext(ctx).Where("run_id = ? AND id > ?", runID, cursor).
		Order("id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}
