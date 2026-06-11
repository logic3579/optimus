package application

import (
	"context"
	"strings"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"optimus-be/internal/models"
)

// Repo is the GORM data-access layer for apps_applications.
type Repo struct{ db *gorm.DB }

// NewRepo returns a Repo bound to the given *gorm.DB.
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// DB exposes the underlying *gorm.DB so tests / future siblings can reach raw rows.
func (r *Repo) DB() *gorm.DB { return r.db }

// Create persists a new application row.
func (r *Repo) Create(ctx context.Context, m *models.AppsApplication) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// Get loads one application by primary key with associations preloaded.
func (r *Repo) Get(ctx context.Context, id uint64) (*models.AppsApplication, error) {
	var m models.AppsApplication
	err := r.db.WithContext(ctx).
		Preload("Cluster").
		Preload("ChartRepo").
		Preload("OwnerUser").
		First(&m, id).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// FindByReleaseTuple looks up an application by the (cluster_id, namespace,
// release_name) triple — the natural key that the partial unique index
// enforces.
func (r *Repo) FindByReleaseTuple(ctx context.Context, clusterID uint64, ns, release string) (*models.AppsApplication, error) {
	var m models.AppsApplication
	err := r.db.WithContext(ctx).
		Where("cluster_id = ? AND namespace = ? AND release_name = ?", clusterID, ns, release).
		First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// FindByName loads the (non-deleted) application with the given name.
func (r *Repo) FindByName(ctx context.Context, name string) (*models.AppsApplication, error) {
	var m models.AppsApplication
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// List returns one page of applications plus the total count matching the filter.
func (r *Repo) List(ctx context.Context, q ListQuery) ([]models.AppsApplication, int64, error) {
	tx := r.db.WithContext(ctx).Model(&models.AppsApplication{}).
		Preload("Cluster").Preload("ChartRepo").Preload("OwnerUser")
	if s := strings.TrimSpace(q.Name); s != "" {
		tx = tx.Where("name ILIKE ?", "%"+s+"%")
	}
	if q.ClusterID != 0 {
		tx = tx.Where("cluster_id = ?", q.ClusterID)
	}
	if ns := strings.TrimSpace(q.Namespace); ns != "" {
		tx = tx.Where("namespace = ?", ns)
	}
	if q.OwnerUserID != 0 {
		tx = tx.Where("owner_user_id = ?", q.OwnerUserID)
	}
	if t := strings.TrimSpace(q.Tag); t != "" {
		// JSONB contains-any: tags @> ["<t>"]. Tag-filter charset is restricted
		// at the handler layer to safe characters before reaching here.
		tx = tx.Where("tags @> ?::jsonb", datatypes.JSON(`["`+t+`"]`))
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
	var rows []models.AppsApplication
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
		Model(&models.AppsApplication{}).
		Where("id = ?", id).
		Updates(fields).Error
}

// Delete soft-deletes by id.
func (r *Repo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&models.AppsApplication{}, id).Error
}

// CountByClusterID counts LIVE (non-soft-deleted) applications that reference
// the given cluster. Used by k8s/cluster.Delete via the Counter seam.
func (r *Repo) CountByClusterID(ctx context.Context, clusterID uint64) (int, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&models.AppsApplication{}).
		Where("cluster_id = ?", clusterID).
		Count(&n).Error
	return int(n), err
}

// CountByChartRepoID counts LIVE applications that reference the given chart
// repo. Used by apps/repo.Delete via the Counter seam.
func (r *Repo) CountByChartRepoID(ctx context.Context, repoID uint64) (int, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&models.AppsApplication{}).
		Where("chart_repo_id = ?", repoID).
		Count(&n).Error
	return int(n), err
}
