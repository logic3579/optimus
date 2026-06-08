package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// LoginLimiter enforces per-IP AND per-username quotas. Allow returns false if either is exhausted.
// In-memory only — single process, no Redis. Sufficient for P0 internal tool.
type LoginLimiter struct {
	quota  int
	window time.Duration
	mu     sync.Mutex
	ip     map[string]*rate.Limiter
	user   map[string]*rate.Limiter
}

// NewLoginLimiter creates a limiter where each (ip OR username) gets `quota` attempts per `window`.
// `block` is reserved for future explicit-block enforcement; not used by the token-bucket impl.
func NewLoginLimiter(quota int, window, block time.Duration) *LoginLimiter {
	_ = block
	return &LoginLimiter{
		quota:  quota,
		window: window,
		ip:     map[string]*rate.Limiter{},
		user:   map[string]*rate.Limiter{},
	}
}

func (l *LoginLimiter) getOrCreate(m map[string]*rate.Limiter, key string) *rate.Limiter {
	if r, ok := m[key]; ok {
		return r
	}
	r := rate.NewLimiter(rate.Limit(float64(l.quota)/l.window.Seconds()), l.quota)
	m[key] = r
	return r
}

// Allow returns true iff both per-IP and per-username buckets currently have a token,
// in which case it consumes one from each. If either is exhausted, returns false and
// does NOT consume from either.
func (l *LoginLimiter) Allow(ip, username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	ipLim := l.getOrCreate(l.ip, ip)
	userLim := l.getOrCreate(l.user, username)

	now := time.Now()
	// Peek both before consuming to avoid one-sided spend.
	if ipLim.TokensAt(now) < 1 || userLim.TokensAt(now) < 1 {
		return false
	}
	// Consume from both — these AllowN calls will succeed because we just peeked.
	if !ipLim.AllowN(now, 1) {
		return false
	}
	if !userLim.AllowN(now, 1) {
		return false
	}
	return true
}
