// Package inuse is the one-way dependency surface from P1 (credentials) into
// P2 (k8s). It exposes nothing but a single counter so the kubeconfig delete
// path can refuse deletes while clusters still reference the kubeconfig.
//
// The helper lives in its own tiny sub-package (no transitive k8s deps) so
// P1's credentials/kubeconfig can import it without dragging the rest of the
// cluster module — which would create a P1 -> P2 -> P1 import cycle.
package inuse

import (
	"context"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

// CountByKubeconfigID returns the number of LIVE (non-soft-deleted) clusters
// that reference the given kubeconfig.
//
// GORM's default scope honors models.Cluster.DeletedAt (gorm.DeletedAt with
// `gorm:"index"`), so soft-deleted rows are filtered out automatically.
func CountByKubeconfigID(ctx context.Context, db *gorm.DB, kubeconfigID uint64) (int64, error) {
	var n int64
	err := db.WithContext(ctx).
		Model(&models.Cluster{}).
		Where("kubeconfig_id = ?", kubeconfigID).
		Count(&n).Error
	return n, err
}
