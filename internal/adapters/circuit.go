package adapters

import (
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// CircuitBreaker isolates per-adapter faults (Mega 509, YouTube 403/429, ADB disconnect).
type CircuitBreaker struct {
	mu           sync.Mutex
	failures     int
	threshold    int
	openUntil    time.Time
	cooldownBase time.Duration
}

// NewCircuitBreaker constructs a circuit breaker opening after threshold consecutive failures.
func NewCircuitBreaker(threshold int, cooldownBase time.Duration) *CircuitBreaker {
	if threshold <= 0 {
		threshold = 3
	}
	if cooldownBase <= 0 {
		cooldownBase = 30 * time.Second
	}
	return &CircuitBreaker{threshold: threshold, cooldownBase: cooldownBase}
}

// Allow reports whether calls to the adapter may proceed.
func (c *CircuitBreaker) Allow(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.openUntil.IsZero() {
		return true
	}
	return !now.Before(c.openUntil)
}

// RecordSuccess resets failure state.
func (c *CircuitBreaker) RecordSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failures = 0
	c.openUntil = time.Time{}
}

// RecordFailure increments failure counter and opens circuit if threshold reached.
func (c *CircuitBreaker) RecordFailure(now time.Time, explicitCooldown time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failures++
	if explicitCooldown > 0 {
		c.openUntil = now.Add(explicitCooldown)
		return
	}
	if c.failures >= c.threshold {
		c.openUntil = now.Add(c.cooldownBase)
	}
}

// Status returns circuit state and remaining cooldown seconds.
func (c *CircuitBreaker) Status(now time.Time) (string, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.openUntil.IsZero() && now.Before(c.openUntil) {
		return "open", int64(c.openUntil.Sub(now).Seconds())
	}
	if c.failures > 0 {
		return "half-open", 0
	}
	return "closed", 0
}

// ComputeBackoff honors Retry-After headers on 429/503/509 or computes jittered exponential backoff.
func ComputeBackoff(resp *http.Response, attempt int) time.Duration {
	if resp != nil {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
				return time.Duration(secs) * time.Second
			}
		}
	}
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 6 {
		attempt = 6
	}
	base := time.Second << attempt
	jitter := time.Duration(rand.Int63n(int64(base/2 + 1)))
	return base + jitter
}
