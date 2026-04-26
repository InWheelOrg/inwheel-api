/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestClientIP_IgnoresXForwardedFor(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "1.2.3.4:9999"
	r.Header.Set("X-Forwarded-For", "9.9.9.9")

	if got := ClientIP(r); got != "1.2.3.4" {
		t.Errorf("ClientIP = %q, want %q (XFF must be ignored)", got, "1.2.3.4")
	}
}

func TestClientIP_SplitsRemoteAddr(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:12345"

	if got := ClientIP(r); got != "10.0.0.1" {
		t.Errorf("ClientIP = %q, want %q", got, "10.0.0.1")
	}
}

func TestRateLimiter_Sweep_EvictsIdleEntries(t *testing.T) {
	// One token refills every 10ms: fast enough for the test, slow enough that
	// the first sweep (called immediately after Allow) sees tokens < burst.
	rl := &RateLimiter{r: rate.Every(10 * time.Millisecond), b: 1}

	rl.Allow("key") // consumes the one token; entry is now active (tokens ≈ 0)
	rl.sweep()      // called immediately — tokens < burst, must NOT evict
	if _, ok := rl.limiters.Load("key"); !ok {
		t.Fatal("sweep evicted an active (non-full) entry")
	}

	time.Sleep(100 * time.Millisecond) // wait for the token to fully refill

	rl.sweep() // tokens == burst — must evict
	if _, ok := rl.limiters.Load("key"); ok {
		t.Fatal("sweep did not evict a fully-replenished idle entry")
	}
}

func TestRateLimiter_Sweep_KeepsActiveEntries(t *testing.T) {
	// Slow refill so tokens cannot recover during the test.
	rl := &RateLimiter{r: rate.Every(time.Hour), b: 5}

	for range 3 {
		rl.Allow("key") // consume 3 of 5 tokens; bucket is not full
	}

	rl.sweep()

	if _, ok := rl.limiters.Load("key"); !ok {
		t.Fatal("sweep evicted an entry that still has active rate limiting")
	}
}

// TestRequireAPIKey_RateLimitBeforeDB passes nil as the DB to prove the DB is never
// reached when the rate limiter drops the request. A nil DB dereference would panic.
func TestRequireAPIKey_RateLimitBeforeDB(t *testing.T) {
	limiter := &RateLimiter{r: rate.Every(time.Hour), b: 0} // burst=0: Allow always false

	handler := RequireAPIKey(nil, limiter, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler must not be called when rate limited before DB lookup")
	})

	r := httptest.NewRequest(http.MethodPost, "/places", nil)
	r.Header.Set("Authorization", "Bearer iwk_somefakekey")
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "rate limit exceeded" {
		t.Errorf("error = %q, want 'rate limit exceeded'", resp["error"])
	}
}
