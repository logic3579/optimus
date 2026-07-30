package advisorylock

import (
	"context"

	"gorm.io/gorm"
)

const applicationLockNamespace = "optimus:application-lifecycle:"

func applicationLockKey(id uint64) int64 {
	return lifecycleLockKey(applicationLockNamespace, id)
}

// LockApplication blocks until the transaction owns the shared application
// lifecycle lock. P3 application deletion and P6 environment binding both use
// this exact helper so their validation and writes cannot pass each other.
func LockApplication(ctx context.Context, tx *gorm.DB, id uint64) error {
	return lockLifecycle(ctx, tx, applicationLockKey(id), "application")
}
