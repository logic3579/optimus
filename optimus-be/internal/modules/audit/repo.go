package audit

import (
	"context"

	"gorm.io/gorm"

	"optimus-be/internal/infra/pagination"
	"optimus-be/internal/models"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) List(ctx context.Context, q ListQuery, p pagination.Params) ([]models.AuditLog, int64, error) {
	tx := r.db.WithContext(ctx).Model(&models.AuditLog{})
	if q.Action != "" {
		tx = tx.Where("action = ?", q.Action)
	}
	if q.UserID != nil {
		tx = tx.Where("user_id = ?", *q.UserID)
	}
	if q.Start != nil {
		tx = tx.Where("created_at >= ?", *q.Start)
	}
	if q.End != nil {
		tx = tx.Where("created_at < ?", *q.End)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.AuditLog
	if err := tx.Order("id DESC").Offset(p.Offset()).Limit(p.Limit()).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
