package sshkey

import (
	"context"
	"strings"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) DB() *gorm.DB { return r.db }

func (r *Repo) Create(ctx context.Context, m *models.CredentialSSHKey) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *Repo) Get(ctx context.Context, id uint64) (*models.CredentialSSHKey, error) {
	var m models.CredentialSSHKey
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repo) FindByName(ctx context.Context, name string) (*models.CredentialSSHKey, error) {
	var m models.CredentialSSHKey
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repo) List(ctx context.Context, q ListQuery) ([]models.CredentialSSHKey, int64, error) {
	tx := r.db.WithContext(ctx).Model(&models.CredentialSSHKey{})
	if s := strings.TrimSpace(q.Q); s != "" {
		pat := "%" + s + "%"
		tx = tx.Where("name ILIKE ? OR description ILIKE ?", pat, pat)
	}
	if s := strings.TrimSpace(q.Username); s != "" {
		tx = tx.Where("username = ?", s)
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
	var rows []models.CredentialSSHKey
	if err := tx.Order("id DESC").
		Limit(q.PageSize).
		Offset((q.Page - 1) * q.PageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Update applies a fields map. Use a nil value for a BYTEA column to clear it.
func (r *Repo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&models.CredentialSSHKey{}).
		Where("id = ?", id).
		Updates(fields).Error
}

func (r *Repo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&models.CredentialSSHKey{}, id).Error
}
