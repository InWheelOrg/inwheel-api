/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/InWheelOrg/inwheel-server/pkg/models"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
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

// RequireAPIKey enforces API key auth and per-key rate limiting before delegating to next.
// Rate-limit check precedes the DB lookup; revoked keys are filtered at the DB level.
func RequireAPIKey(db *gorm.DB, krl *RateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		rawKey := strings.TrimPrefix(authHeader, "Bearer ")
		hash := SHA256Hex(rawKey)

		if !krl.Allow(hash) {
			w.Header().Set("Retry-After", strconv.Itoa(krl.RetryAfterSeconds()))
			jsonError(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		var apiKey models.APIKey
		if err := db.Where("key_hash = ? AND revoked_at IS NULL", hash).First(&apiKey).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				slog.Error("auth: key lookup failed", "error", err)
				jsonError(w, "internal server error", http.StatusInternalServerError)
				return
			}
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
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

// jsonError writes a minimal {"error": msg} JSON response.
func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	payload, _ := json.Marshal(map[string]string{"error": msg})
	if _, err := w.Write(payload); err != nil {
		slog.Error("Error writing JSON error response", "error", err)
	}
}
