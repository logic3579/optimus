package advisorylock

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCloudKeyLockKeyIsDeterministicAndSignedSafe(t *testing.T) {
	require.Equal(t, cloudKeyLockKey(42), cloudKeyLockKey(42))
	require.NotEqual(t, cloudKeyLockKey(42), cloudKeyLockKey(43))
	require.GreaterOrEqual(t, cloudKeyLockKey(math.MaxUint64), int64(0))
}
