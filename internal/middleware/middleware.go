/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/InWheelOrg/inwheel-server/pkg/models"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

// RateLimiter holds per-key token buckets keyed by an arbitrary string (IP, key hash, etc.).
// Safe for concurrent use. A background goroutine evicts idle entries every 5 minutes to
// prevent unbounded memory growth from unique IPs or keys that never repeat.
type RateLimiter struct {
	limiters sync.Map
	r        rate.Limit
	b        int
}

// NewRateLimiter creates a RateLimiter with the given steady-state rate and burst size.
// It starts a background goroutine to periodically evict fully-replenished (idle) entries.
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	rl := &RateLimiter{r: r, b: b}
	go rl.evictIdle()
	return rl
}

// evictIdle periodically removes entries whose limiter is fully replenished, indicating
// the key has been idle long enough to have earned back all tokens.
func (l *RateLimiter) evictIdle() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		l.sweep()
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

// Allow reports whether the given key has a token available.
func (l *RateLimiter) Allow(key string) bool {
	if v, ok := l.limiters.Load(key); ok {
		return v.(*rate.Limiter).Allow()
	}
	v, _ := l.limiters.LoadOrStore(key, rate.NewLimiter(l.r, l.b))
	return v.(*rate.Limiter).Allow()
}

// RequireAPIKey returns a handler that enforces API key authentication and per-key
// rate limiting before delegating to next. The rate limit check runs before the DB
// lookup to avoid unnecessary queries under a flood of valid-key requests.
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
			jsonError(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		var apiKey models.APIKey
		if err := db.Where("key_hash = ?", hash).First(&apiKey).Error; err != nil {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if apiKey.RevokedAt != nil {
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
