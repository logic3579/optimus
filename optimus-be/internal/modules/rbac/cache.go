package rbac

import (
	"context"
	"sync"
	"time"

	"gorm.io/gorm"
)

type cacheEntry struct {
	codes  []string
	stored time.Time
}

// PermissionCache caches user → permission code list with a TTL.
// Single-process; no cross-instance invalidation. Sufficient for P0.
type PermissionCache struct {
	db  *gorm.DB
	ttl time.Duration
	m   sync.Map // map[uint64]cacheEntry
}

func NewPermissionCache(db *gorm.DB, ttl time.Duration) *PermissionCache {
	return &PermissionCache{db: db, ttl: ttl}
}

// Get returns permission codes for the user. Hits cache if within TTL, else queries DB.
func (p *PermissionCache) Get(ctx context.Context, userID uint64) ([]string, error) {
	if v, ok := p.m.Load(userID); ok {
		e := v.(cacheEntry)
		if time.Since(e.stored) < p.ttl {
			return e.codes, nil
		}
	}
	codes, err := p.load(ctx, userID)
	if err != nil {
		return nil, err
	}
	p.m.Store(userID, cacheEntry{codes: codes, stored: time.Now()})
	return codes, nil
}

// InvalidateUser drops a user from the cache. Call after a role/perm change for that user.
func (p *PermissionCache) InvalidateUser(userID uint64) { p.m.Delete(userID) }

// InvalidateAll clears the entire cache.
func (p *PermissionCache) InvalidateAll() {
	p.m.Range(func(k, _ any) bool { p.m.Delete(k); return true })
}

func (p *PermissionCache) load(ctx context.Context, userID uint64) ([]string, error) {
	var codes []string
	err := p.db.WithContext(ctx).
		Table("permissions p").
		Select("DISTINCT p.code").
		Joins("JOIN role_permissions rp ON rp.permission_id = p.id").
		Joins("JOIN user_roles ur ON ur.role_id = rp.role_id").
		Joins("JOIN users u ON u.id = ur.user_id").
		Where("u.id = ? AND u.deleted_at IS NULL", userID).
		Where("EXISTS (SELECT 1 FROM roles r WHERE r.id = ur.role_id AND r.deleted_at IS NULL)").
		Pluck("p.code", &codes).Error
	if err != nil {
		return nil, err
	}
	return codes, nil
}
