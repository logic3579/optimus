package httpcredential

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }
func (r *Repo) Create(ctx context.Context, m *models.HTTPCredential) error {
	return mapWriteError(r.db.WithContext(ctx).Create(m).Error)
}
func (r *Repo) Get(ctx context.Context, id uint64) (*models.HTTPCredential, error) {
	var m models.HTTPCredential
	err := r.db.WithContext(ctx).First(&m, id).Error
	return &m, err
}
func (r *Repo) FindByName(ctx context.Context, n string) (*models.HTTPCredential, error) {
	var m models.HTTPCredential
	err := r.db.WithContext(ctx).Where("name = ?", n).First(&m).Error
	return &m, err
}
func (r *Repo) List(ctx context.Context, q ListQuery) ([]models.HTTPCredential, int64, error) {
	tx := r.db.WithContext(ctx).Model(&models.HTTPCredential{})
	if s := strings.TrimSpace(q.Q); s != "" {
		tx = tx.Where("name ILIKE ?", "%"+s+"%")
	}
	if q.AuthType != "" {
		tx = tx.Where("auth_type = ?", q.AuthType)
	}
	var n int64
	if err := tx.Count(&n).Error; err != nil {
		return nil, 0, err
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	var out []models.HTTPCredential
	err := tx.Order("id DESC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&out).Error
	return out, n, err
}
func (r *Repo) Update(ctx context.Context, id uint64, f map[string]any) error {
	return mapWriteError(r.db.WithContext(ctx).Model(&models.HTTPCredential{}).Where("id = ?", id).Updates(f).Error)
}
func mapWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "credentials_http_name_unique" {
		return conflict()
	}
	return err
}
func (r *Repo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&models.HTTPCredential{}, id).Error
}
