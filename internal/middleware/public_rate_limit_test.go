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

func TestPublicRateLimit_SkipsAuthenticatedRequests(t *testing.T) {
	t.Parallel()
	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ })

	rl := &RateLimiter{r: rate.Every(time.Hour), b: 1}
	handler := PublicRateLimit(rl)(next)

	authed := func() *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/v1/places", nil)
		r.RemoteAddr = "1.2.3.4:9999"
		return r.WithContext(WithAPIKeyID(r.Context(), "key-id"))
	}

	handler.ServeHTTP(httptest.NewRecorder(), authed())
	handler.ServeHTTP(httptest.NewRecorder(), authed())

	if calls != 2 {
		t.Fatalf("next was called %d times, want 2 (authenticated requests bypass the read limiter)", calls)
	}
}

func TestPublicRateLimit_DoesNotTrustUnverifiedAPIKeyHeader(t *testing.T) {
	t.Parallel()
	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ })

	rl := &RateLimiter{r: rate.Every(time.Hour), b: 1}
	handler := PublicRateLimit(rl)(next)

	unauthed := func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/v1/places", nil)
		r.RemoteAddr = "1.2.3.4:9999"
		r.Header.Set("X-API-Key", "garbage")
		return r
	}

	handler.ServeHTTP(httptest.NewRecorder(), unauthed()) // consumes the one token
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, unauthed())

	if calls != 1 {
		t.Fatalf("next was called %d times, want 1 (a bare X-API-Key header must not bypass the limiter)", calls)
	}
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}
