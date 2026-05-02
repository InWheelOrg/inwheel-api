/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter holds per-key token buckets keyed by an arbitrary string (IP, key hash, etc.).
// Safe for concurrent use. A background goroutine evicts idle entries every 5 minutes to
// cap memory growth from unique keys; it exits when the context is cancelled.
type RateLimiter struct {
	limiters sync.Map
	r        rate.Limit
	b        int
	done     chan struct{} // closed once the eviction goroutine returns
}

// NewRateLimiter creates a RateLimiter with the given rate and burst. The eviction
// goroutine runs until ctx is cancelled.
func NewRateLimiter(ctx context.Context, r rate.Limit, b int) *RateLimiter {
	rl := &RateLimiter{r: r, b: b, done: make(chan struct{})}
	go rl.evictIdle(ctx)
	return rl
}

// evictIdle periodically removes fully-replenished entries until ctx is cancelled.
func (l *RateLimiter) evictIdle(ctx context.Context) {
	defer close(l.done)
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.sweep()
		case <-ctx.Done():
			return
		}
	}
}

// sweep removes all entries whose token bucket is fully replenished (idle keys).
func (l *RateLimiter) sweep() {
	l.limiters.Range(func(k, v any) bool {
		if v.(*rate.Limiter).Tokens() >= float64(l.b) {
			l.limiters.Delete(k)
		}
		return true
	})
}

// RetryAfterSeconds returns the number of seconds until the next token is available
// (ceil of the refill period). Use this to set the Retry-After response header.
func (l *RateLimiter) RetryAfterSeconds() int {
	if l.r <= 0 {
		return 0
	}
	return int(math.Ceil(1 / float64(l.r)))
}

// Allow reports whether the given key has a token available.
func (l *RateLimiter) Allow(key string) bool {
	if v, ok := l.limiters.Load(key); ok {
		return v.(*rate.Limiter).Allow()
	}
	v, _ := l.limiters.LoadOrStore(key, rate.NewLimiter(l.r, l.b))
	return v.(*rate.Limiter).Allow()
}

// SHA256Hex returns the lowercase hex-encoded SHA-256 digest of s.
func SHA256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// ClientIP extracts the real client IP from the request.
// Uses RemoteAddr only; X-Forwarded-For is caller-controlled and cannot
// be trusted for rate-limiting without a trusted-proxy allowlist.
func ClientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
