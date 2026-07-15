// Package inuse exposes the cloud-account reference counter consumed by the
// credentials cloud-key delete path without importing the assets CRUD module.
package inuse

import (
	"context"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type Counter interface {
	CountByCloudKeyID(ctx context.Context, cloudKeyID uint64) (int64, error)
}

type GORMCounter struct {
	db *gorm.DB
}

func New(db *gorm.DB) *GORMCounter { return &GORMCounter{db: db} }

func (c *GORMCounter) CountByCloudKeyID(ctx context.Context, cloudKeyID uint64) (int64, error) {
	var count int64
	err := c.db.WithContext(ctx).
		Model(&models.CloudAccount{}).
		Where("cloudkey_id = ?", cloudKeyID).
		Count(&count).Error
	return count, err
}

var _ Counter = (*GORMCounter)(nil)
