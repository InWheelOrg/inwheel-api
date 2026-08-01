/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestPublicRateLimit_AllowsUnderLimit(t *testing.T) {
	t.Parallel()
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	rl := &RateLimiter{r: 1, b: 1}
	handler := PublicRateLimit(rl)(next)
	r := httptest.NewRequest(http.MethodGet, "/v1/places", nil)
	r.RemoteAddr = "1.2.3.4:9999"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("next was not called")
	}
}

func TestPublicRateLimit_BlocksOverLimit(t *testing.T) {
	t.Parallel()
	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ })

	rl := &RateLimiter{r: rate.Every(time.Hour), b: 1}
	handler := PublicRateLimit(rl)(next)

	r1 := httptest.NewRequest(http.MethodGet, "/v1/places", nil)
	r1.RemoteAddr = "1.2.3.4:9999"
	handler.ServeHTTP(httptest.NewRecorder(), r1) // consumes the one token

	r2 := httptest.NewRequest(http.MethodGet, "/v1/places", nil)
	r2.RemoteAddr = "1.2.3.4:9999"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r2)

	if calls != 1 {
		t.Fatalf("next was called %d times, want 1 (only the first request)", calls)
	}
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

func TestPublicRateLimit_SkipsRequestsWithAPIKey(t *testing.T) {
	t.Parallel()
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	rl := &RateLimiter{r: rate.Every(time.Hour), b: 1}
	handler := PublicRateLimit(rl)(next)

	r1 := httptest.NewRequest(http.MethodPost, "/v1/places", nil)
	r1.RemoteAddr = "1.2.3.4:9999"
	r1.Header.Set("X-API-Key", "iwk_test")
	handler.ServeHTTP(httptest.NewRecorder(), r1)

	r2 := httptest.NewRequest(http.MethodPost, "/v1/places", nil)
	r2.RemoteAddr = "1.2.3.4:9999"
	r2.Header.Set("X-API-Key", "iwk_test")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r2)

	if !called {
		t.Fatal("next was not called for a request carrying X-API-Key")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (no status set by middleware)", w.Code, http.StatusOK)
	}
}
