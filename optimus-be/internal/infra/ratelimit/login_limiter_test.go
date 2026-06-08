package ratelimit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/ratelimit"
)

func TestLoginLimiter_AllowsUnderQuota(t *testing.T) {
	lim := ratelimit.NewLoginLimiter(3, time.Minute, time.Minute)
	for i := 0; i < 3; i++ {
		require.True(t, lim.Allow("1.2.3.4", "alice"), "attempt %d should be allowed", i+1)
	}
}

func TestLoginLimiter_BlocksAfterIPQuota(t *testing.T) {
	lim := ratelimit.NewLoginLimiter(3, time.Minute, time.Minute)
	for i := 0; i < 3; i++ {
		lim.Allow("1.2.3.4", "alice")
	}
	// 4th attempt from same IP, even with different username, blocked
	require.False(t, lim.Allow("1.2.3.4", "bob"))
}

func TestLoginLimiter_BlocksAfterUsernameQuota(t *testing.T) {
	lim := ratelimit.NewLoginLimiter(3, time.Minute, time.Minute)
	for i := 0; i < 3; i++ {
		lim.Allow("1.2.3.4", "alice")
		lim.Allow("9.9.9.9", "alice")
	}
	require.False(t, lim.Allow("5.5.5.5", "alice"), "should be blocked by username quota")
}

func TestLoginLimiter_IsolatesDifferentKeys(t *testing.T) {
	lim := ratelimit.NewLoginLimiter(2, time.Minute, time.Minute)
	require.True(t, lim.Allow("1.1.1.1", "alice"))
	require.True(t, lim.Allow("1.1.1.1", "alice"))
	require.False(t, lim.Allow("1.1.1.1", "alice"))
	// Different IP + username pair is independent
	require.True(t, lim.Allow("2.2.2.2", "bob"))
}
