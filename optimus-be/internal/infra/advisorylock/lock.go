package advisorylock

import (
	"context"
	"encoding/binary"
	"errors"
	"hash/fnv"
	"math"

	"gorm.io/gorm"
)

func lifecycleLockKey(namespace string, id uint64) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(namespace))
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], id)
	_, _ = h.Write(encoded[:])
	return int64(h.Sum64() & uint64(math.MaxInt64))
}

func lockLifecycle(ctx context.Context, tx *gorm.DB, key int64, resource string) error {
	if tx == nil {
		return errors.New(resource + " advisory lock requires a transaction")
	}
	return tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(?)", key).Error
}
