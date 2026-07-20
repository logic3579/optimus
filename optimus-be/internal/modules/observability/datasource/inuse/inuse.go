package inuse

import (
	"context"
	"gorm.io/gorm"
	"optimus-be/internal/models"
)

type Counter struct{ db *gorm.DB }

func New(db *gorm.DB) *Counter { return &Counter{db: db} }
func (c *Counter) CountByHTTPCredentialID(ctx context.Context, id uint64) (int64, error) {
	return countCredential(ctx, c.db, id)
}
func (c *Counter) CountByHTTPCredentialIDTx(ctx context.Context, tx *gorm.DB, id uint64) (int64, error) {
	return countCredential(ctx, tx, id)
}
func countCredential(ctx context.Context, db *gorm.DB, id uint64) (int64, error) {
	var n int64
	err := db.WithContext(ctx).Model(&models.ObservabilityDatasource{}).Where("http_credential_id=?", id).Count(&n).Error
	return n, err
}
func (c *Counter) CountByClusterID(ctx context.Context, id uint64) (int64, error) {
	return countCluster(ctx, c.db, id)
}
func (c *Counter) CountByClusterIDTx(ctx context.Context, tx *gorm.DB, id uint64) (int64, error) {
	return countCluster(ctx, tx, id)
}
func countCluster(ctx context.Context, db *gorm.DB, id uint64) (int64, error) {
	var n int64
	err := db.WithContext(ctx).Model(&models.ObservabilityDatasource{}).Where("cluster_id=?", id).Count(&n).Error
	return n, err
}
