package cluster

import (
	"context"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) DB() *gorm.DB { return r.db }

func (r *Repo) Create(ctx context.Context, m *models.Cluster) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *Repo) Get(ctx context.Context, id uint64) (*models.Cluster, error) {
	var m models.Cluster
	if err := r.db.WithContext(ctx).Preload("Kubeconfig").First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repo) FindByName(ctx context.Context, name string) (*models.Cluster, error) {
	var m models.Cluster
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repo) List(ctx context.Context, q ListQuery) ([]models.Cluster, int64, error) {
	tx := r.db.WithContext(ctx).Model(&models.Cluster{}).Preload("Kubeconfig")
	if s := strings.TrimSpace(q.Search); s != "" {
		pat := "%" + s + "%"
		tx = tx.Where("name ILIKE ? OR description ILIKE ?", pat, pat)
	}
	if t := strings.TrimSpace(q.Tag); t != "" {
		// JSONB contains-any: tags @> ["<t>"]. The handler restricts q.Tag to
		// [a-zA-Z0-9_-]+ before reaching here, so the inline JSON construction
		// is safe; we still build the operand via datatypes.JSON to give GORM
		// a typed parameter rather than raw string interpolation.
		tx = tx.Where("tags @> ?::jsonb", datatypes.JSON(`["`+t+`"]`))
	}
	if q.KubeconfigID != 0 {
		tx = tx.Where("kubeconfig_id = ?", q.KubeconfigID)
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
	var rows []models.Cluster
	if err := tx.Order("id DESC").
		Limit(q.PageSize).
		Offset((q.Page - 1) * q.PageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Repo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&models.Cluster{}).
		Where("id = ?", id).
		Updates(fields).Error
}

func (r *Repo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&models.Cluster{}, id).Error
}

func (r *Repo) UpdateHealth(ctx context.Context, id uint64, ok bool, msg string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&models.Cluster{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"last_health_at":  &now,
			"last_health_ok":  &ok,
			"last_health_msg": msg,
		}).Error
}

// CountByKubeconfigID reports how many live clusters reference the given
// kubeconfig. Used by P1's kubeconfig delete handler (via the inuse package)
// to gate DELETE.
func (r *Repo) CountByKubeconfigID(ctx context.Context, kubeconfigID uint64) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).
		Model(&models.Cluster{}).
		Where("kubeconfig_id = ?", kubeconfigID).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}
