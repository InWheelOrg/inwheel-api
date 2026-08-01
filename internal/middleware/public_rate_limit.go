/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
)

func PublicRateLimit(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if APIKeyIDFromCtx(r.Context()) == "" && !rl.Allow(ClientIP(r)) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", strconv.Itoa(rl.RetryAfterSeconds()))
				w.WriteHeader(http.StatusTooManyRequests)
				if err := json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"}); err != nil {
					slog.Error("PublicRateLimit: encode failed", "error", err)
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
