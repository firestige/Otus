// Package decoder implements protocol decoding.
package decoder

import (
	"sync"
	"sync/atomic"
	"time"
)

// FragmentRateLimiter tracks per-source-IP fragment counts to prevent
// fragment flood DoS attacks. It uses a sliding window approach: counts
// are stored per window and automatically rotated.
type FragmentRateLimiter struct {
	mu           sync.Mutex
	current      map[[4]byte]*atomic.Int64 // source IP → fragment count in current window
	windowStart  time.Time
	windowSize   time.Duration
	maxPerWindow int64

	// Metrics
	rejected atomic.Int64 // total rejected fragments
}

// FragmentRateLimiterConfig configures per-IP fragment rate limiting.
type FragmentRateLimiterConfig struct {
	MaxFragsPerIP   int           // Max fragments per source IP per window (0 = disabled)
	RateLimitWindow time.Duration // Window size (default 10s)
}

// NewFragmentRateLimiter creates a rate limiter. Returns nil if disabled (MaxFragsPerIP <= 0).
func NewFragmentRateLimiter(cfg FragmentRateLimiterConfig) *FragmentRateLimiter {
	if cfg.MaxFragsPerIP <= 0 {
		return nil
	}
	if cfg.RateLimitWindow <= 0 {
		cfg.RateLimitWindow = 10 * time.Second
	}
	return &FragmentRateLimiter{
		current:      make(map[[4]byte]*atomic.Int64),
		windowStart:  time.Now(),
		windowSize:   cfg.RateLimitWindow,
		maxPerWindow: int64(cfg.MaxFragsPerIP),
	}
}

// Allow checks if a fragment from the given source IP is allowed.
// Returns true if allowed, false if rate-limited.
func (l *FragmentRateLimiter) Allow(srcIP [4]byte, now time.Time) bool {
	l.mu.Lock()

	// Rotate window if expired
	if now.Sub(l.windowStart) >= l.windowSize {
		l.current = make(map[[4]byte]*atomic.Int64)
		l.windowStart = now
	}

	counter, exists := l.current[srcIP]
	if !exists {
		counter = &atomic.Int64{}
		l.current[srcIP] = counter
	}
	l.mu.Unlock()

	// Atomic increment + check (lock-free hot path after map lookup)
	count := counter.Add(1)
	if count > l.maxPerWindow {
		l.rejected.Add(1)
		return false
	}
	return true
}

// Rejected returns the total number of rejected fragments.
func (l *FragmentRateLimiter) Rejected() int64 {
	return l.rejected.Load()
}

// ActiveIPs returns the number of distinct source IPs in the current window.
func (l *FragmentRateLimiter) ActiveIPs() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.current)
}
