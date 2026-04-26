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

	"github.com/InWheelOrg/inwheel-server/pkg/models"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

// RateLimiter holds per-key token buckets keyed by an arbitrary string (IP, key hash, etc.).
// Safe for concurrent use.
type RateLimiter struct {
	limiters sync.Map
	r        rate.Limit
	b        int
}

// NewRateLimiter creates a RateLimiter with the given steady-state rate and burst size.
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	return &RateLimiter{r: r, b: b}
}

// Allow reports whether the given key has a token available.
func (l *RateLimiter) Allow(key string) bool {
	v, _ := l.limiters.LoadOrStore(key, rate.NewLimiter(l.r, l.b))
	return v.(*rate.Limiter).Allow()
}

// RequireAPIKey returns a handler that enforces API key authentication and per-key
// rate limiting before delegating to next.
func RequireAPIKey(db *gorm.DB, krl *RateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		rawKey := strings.TrimPrefix(authHeader, "Bearer ")
		hash := SHA256Hex(rawKey)

		var apiKey models.APIKey
		if err := db.Where("key_hash = ?", hash).First(&apiKey).Error; err != nil {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if apiKey.RevokedAt != nil {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !krl.Allow(hash) {
			jsonError(w, "rate limit exceeded", http.StatusTooManyRequests)
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
// Checks X-Forwarded-For first, falls back to RemoteAddr.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
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
