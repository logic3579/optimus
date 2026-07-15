// Package advisorylock coordinates lifecycle mutations that span modules but
// must serialize on the same PostgreSQL transaction-scoped lock.
package advisorylock

import (
	"context"
	"encoding/binary"
	"errors"
	"hash/fnv"
	"math"

	"gorm.io/gorm"
)

const cloudKeyLockNamespace = "optimus:cloud-key-lifecycle:"

// cloudKeyLockKey hashes the namespace and full uint64 ID, then masks the sign
// bit so no direct uint64-to-int64 overflow is possible. Hash collisions only
// serialize unrelated keys conservatively; equal IDs always produce equal keys.
func cloudKeyLockKey(id uint64) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(cloudKeyLockNamespace))
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], id)
	_, _ = h.Write(encoded[:])
	return int64(h.Sum64() & uint64(math.MaxInt64))
}

// LockCloudKey blocks until the transaction owns the cloud-key lifecycle lock.
// PostgreSQL releases the lock automatically on commit or rollback.
func LockCloudKey(ctx context.Context, tx *gorm.DB, id uint64) error {
	if tx == nil {
		return errors.New("cloud key advisory lock requires a transaction")
	}
	return tx.WithContext(ctx).
		Exec("SELECT pg_advisory_xact_lock(?)", cloudKeyLockKey(id)).Error
}
