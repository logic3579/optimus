package repo

import (
	"context"
	"strings"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

// Repo is the GORM data-access layer for apps_chart_repos.
type Repo struct{ db *gorm.DB }

// NewRepo returns a Repo bound to the given *gorm.DB.
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// DB exposes the underlying *gorm.DB so tests / future siblings can reach raw rows.
func (r *Repo) DB() *gorm.DB { return r.db }

// Create persists a new chart repo row.
func (r *Repo) Create(ctx context.Context, m *models.AppsChartRepo) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// Get loads one chart repo by primary key.
func (r *Repo) Get(ctx context.Context, id uint64) (*models.AppsChartRepo, error) {
	var m models.AppsChartRepo
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// FindByName loads the (non-deleted) repo with the given name.
func (r *Repo) FindByName(ctx context.Context, name string) (*models.AppsChartRepo, error) {
	var m models.AppsChartRepo
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// List returns one page of chart repos plus the total count matching the filter.
func (r *Repo) List(ctx context.Context, q ListQuery) ([]models.AppsChartRepo, int64, error) {
	tx := r.db.WithContext(ctx).Model(&models.AppsChartRepo{})
	if s := strings.TrimSpace(q.Name); s != "" {
		tx = tx.Where("name ILIKE ?", "%"+s+"%")
	}
	if t := strings.TrimSpace(q.Type); t != "" {
		tx = tx.Where("type = ?", t)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	var rows []models.AppsChartRepo
	if err := tx.Order("id DESC").
		Limit(q.PageSize).
		Offset((q.Page - 1) * q.PageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Update applies the given column->value map to the row identified by id.
// A nil/empty map is a no-op.
func (r *Repo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&models.AppsChartRepo{}).
		Where("id = ?", id).
		Updates(fields).Error
}

// Delete soft-deletes by id. Caller must run the InUseCounter pre-check.
func (r *Repo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&models.AppsChartRepo{}, id).Error
}
