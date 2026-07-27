// Package advisorylock coordinates lifecycle mutations that span modules but
// must serialize on the same PostgreSQL transaction-scoped lock.
package advisorylock

import (
	"context"

	"gorm.io/gorm"
)

const cloudKeyLockNamespace = "optimus:cloud-key-lifecycle:"

// cloudKeyLockKey hashes the namespace and full uint64 ID, then masks the sign
// bit so no direct uint64-to-int64 overflow is possible. Hash collisions only
// serialize unrelated keys conservatively; equal IDs always produce equal keys.
func cloudKeyLockKey(id uint64) int64 {
	return lifecycleLockKey(cloudKeyLockNamespace, id)
}

// LockCloudKey blocks until the transaction owns the cloud-key lifecycle lock.
// PostgreSQL releases the lock automatically on commit or rollback.
func LockCloudKey(ctx context.Context, tx *gorm.DB, id uint64) error {
	return lockLifecycle(ctx, tx, cloudKeyLockKey(id), "cloud key")
}
