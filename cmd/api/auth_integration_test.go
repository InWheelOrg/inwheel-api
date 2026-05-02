//go:build integration

/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/InWheelOrg/inwheel-server/internal/middleware"
	"github.com/InWheelOrg/inwheel-server/pkg/models"
	"golang.org/x/time/rate"
)

// registerKey is a test helper that registers an API key for the given email
// and returns the raw key string.
func registerKey(t *testing.T, srv *Server, email string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email})
	r := httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlerForServer(t, srv).ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("registerKey: status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("registerKey: decode response: %v", err)
	}
	return resp["api_key"]
}

// --- Registration tests ---

func TestHandleRegister_Success(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	body, _ := json.Marshal(map[string]string{"email": "user@example.com"})
	r := httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(resp["api_key"], "iwk_") {
		t.Errorf("api_key %q does not start with iwk_", resp["api_key"])
	}
	if len(resp["api_key"]) != 68 { // "iwk_" (4) + 64 hex chars
		t.Errorf("api_key length = %d, want 68", len(resp["api_key"]))
	}
	if resp["email"] == "" {
		t.Error("expected non-empty email in response")
	}
}

func TestHandleRegister_DuplicateActiveKey(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	srv := newTestServer(t)
	registerKey(t, srv, "dup@example.com")

	body, _ := json.Marshal(map[string]string{"email": "dup@example.com"})
	r := httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlerForServer(t, srv).ServeHTTP(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleRegister_InvalidEmail(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	body, _ := json.Marshal(map[string]string{"email": "not-an-email"})
	r := httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Fields []struct {
			Field  string `json:"field"`
			Reason string `json:"reason"`
		} `json:"fields"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Fields) == 0 || resp.Fields[0].Field != "email" {
		t.Errorf("expected field error on 'email', got %+v", resp.Fields)
	}
}

func TestHandleRegister_IPRateLimit(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	// burst of 2: first two requests succeed, third is rate-limited
	ctx := t.Context()
	srv := &Server{
		db:         testDB,
		engine:     newTestServer(t).engine,
		regLimiter: middleware.NewRateLimiter(ctx, rate.Every(24*time.Hour), 2),
		keyLimiter: middleware.NewRateLimiter(ctx, rate.Every(time.Millisecond), 1000),
	}

	emails := []string{"a@example.com", "b@example.com", "c@example.com"}
	for i, email := range emails {
		body, _ := json.Marshal(map[string]string{"email": email})
		r := httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handlerForServer(t, srv).ServeHTTP(w, r)

		if i < 2 {
			if w.Code != http.StatusCreated {
				t.Fatalf("request %d: status = %d, want 201", i+1, w.Code)
			}
		} else {
			if w.Code != http.StatusTooManyRequests {
				t.Fatalf("request %d: status = %d, want 429", i+1, w.Code)
			}
		}
	}
}

// --- Auth on write endpoints ---

func TestHandlePostPlace_WithValidKey(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	srv := newTestServer(t)
	rawKey := registerKey(t, srv, "writer@example.com")

	body, _ := json.Marshal(models.Place{
		Name:     "Test Place",
		Lat:      60.1,
		Lng:      24.9,
		Category: models.CategoryCafe,
		Rank:     models.RankEstablishment,
	})
	r := httptest.NewRequest(http.MethodPost, "/v1/places", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-API-Key", rawKey)
	w := httptest.NewRecorder()

	handlerForServer(t, srv).ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
}

func TestHandlePostPlace_MissingAuth(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	srv := newTestServer(t)
	body, _ := json.Marshal(models.Place{Name: "x", Lat: 60.1, Lng: 24.9, Category: models.CategoryCafe, Rank: models.RankEstablishment})
	r := httptest.NewRequest(http.MethodPost, "/v1/places", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handlerForServer(t, srv).ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "unauthorized" {
		t.Errorf("error = %q, want 'unauthorized'", resp["error"])
	}
}

func TestHandlePostPlace_InvalidKey(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	srv := newTestServer(t)
	body, _ := json.Marshal(models.Place{Name: "x", Lat: 60.1, Lng: 24.9, Category: models.CategoryCafe, Rank: models.RankEstablishment})
	r := httptest.NewRequest(http.MethodPost, "/v1/places", bytes.NewReader(body))
	r.Header.Set("X-API-Key", "iwk_notavalidkey000000000000000000000000000000000000000000000000000")
	w := httptest.NewRecorder()

	handlerForServer(t, srv).ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", w.Code, w.Body.String())
	}
}

func TestHandlePostPlace_RevokedKey(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	srv := newTestServer(t)
	rawKey := registerKey(t, srv, "revoked@example.com")

	// Revoke the key directly in the DB.
	now := time.Now()
	testDB.Model(&models.APIKey{}).Where("email = ?", "revoked@example.com").Update("revoked_at", now)

	body, _ := json.Marshal(models.Place{Name: "x", Lat: 60.1, Lng: 24.9, Category: models.CategoryCafe, Rank: models.RankEstablishment})
	r := httptest.NewRequest(http.MethodPost, "/v1/places", bytes.NewReader(body))
	r.Header.Set("X-API-Key", rawKey)
	w := httptest.NewRecorder()

	handlerForServer(t, srv).ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", w.Code, w.Body.String())
	}
}

func TestHandlePatchAccessibility_WithValidKey(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	srv := newTestServer(t)

	// Create a place first (directly in DB).
	place := models.Place{Name: "A Place", Lat: 60.1, Lng: 24.9, Category: models.CategoryCafe, Rank: models.RankEstablishment}
	testDB.Create(&place)

	rawKey := registerKey(t, srv, "patcher@example.com")

	profile := models.AccessibilityProfile{OverallStatus: models.StatusAccessible}
	body, _ := json.Marshal(profile)
	r := httptest.NewRequest(http.MethodPatch, "/v1/places/"+place.ID+"/accessibility", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-API-Key", rawKey)
	r.SetPathValue("id", place.ID)
	w := httptest.NewRecorder()

	handlerForServer(t, srv).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestHandlePatchAccessibility_MissingAuth(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	srv := newTestServer(t)
	r := httptest.NewRequest(http.MethodPatch, "/v1/places/some-id/accessibility", strings.NewReader(`{"overall_status":"accessible"}`))
	w := httptest.NewRecorder()

	handlerForServer(t, srv).ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", w.Code, w.Body.String())
	}
}

// TestHandleRegister_XFFDoesNotBypassRateLimit verifies that rotating X-Forwarded-For
// values cannot defeat the per-IP registration rate limit. All requests share the same
// RemoteAddr, so different XFF headers must have no effect.
func TestHandleRegister_XFFDoesNotBypassRateLimit(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	ctx := t.Context()
	srv := &Server{
		db:         testDB,
		engine:     newTestServer(t).engine,
		regLimiter: middleware.NewRateLimiter(ctx, rate.Every(24*time.Hour), 1),
		keyLimiter: middleware.NewRateLimiter(ctx, rate.Every(time.Millisecond), 1000),
	}

	send := func(email, xff string) int {
		body, _ := json.Marshal(map[string]string{"email": email})
		r := httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.RemoteAddr = "1.2.3.4:9999"
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		w := httptest.NewRecorder()
		handlerForServer(t, srv).ServeHTTP(w, r)
		return w.Code
	}

	if code := send("first@example.com", ""); code != http.StatusCreated {
		t.Fatalf("first request: status = %d, want 201", code)
	}
	// Spoofed XFF, same RemoteAddr — must still hit the rate limit.
	if code := send("second@example.com", "9.9.9.9"); code != http.StatusTooManyRequests {
		t.Fatalf("second request with spoofed XFF: status = %d, want 429", code)
	}
}

// --- Revocation tests ---

func TestHandleRevokeKey_Success(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	srv := newTestServer(t)
	rawKey := registerKey(t, srv, "revoke@example.com")

	r := httptest.NewRequest(http.MethodDelete, "/v1/auth/keys", nil)
	r.Header.Set("X-API-Key", rawKey)
	w := httptest.NewRecorder()
	handlerForServer(t, srv).ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("revoke: status = %d, want 204; body: %s", w.Code, w.Body.String())
	}

	// Revoked key must no longer authenticate.
	body, _ := json.Marshal(models.Place{Name: "X", Lat: 60.1, Lng: 24.9, Category: models.CategoryCafe, Rank: models.RankEstablishment})
	r = httptest.NewRequest(http.MethodPost, "/v1/places", bytes.NewReader(body))
	r.Header.Set("X-API-Key", rawKey)
	w = httptest.NewRecorder()
	handlerForServer(t, srv).ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("revoked key: status = %d, want 401", w.Code)
	}
}

func TestHandleRevokeKey_Unauthenticated(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	r := httptest.NewRequest(http.MethodDelete, "/v1/auth/keys", nil)
	w := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestHandleRevokeKey_AlreadyRevoked(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	srv := newTestServer(t)
	rawKey := registerKey(t, srv, "revoke2@example.com")

	now := time.Now()
	testDB.Model(&models.APIKey{}).Where("email = ?", "revoke2@example.com").Update("revoked_at", now)

	r := httptest.NewRequest(http.MethodDelete, "/v1/auth/keys", nil)
	r.Header.Set("X-API-Key", rawKey)
	w := httptest.NewRecorder()
	handlerForServer(t, srv).ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", w.Code, w.Body.String())
	}
}

// TestHandleRegister_ConcurrentSameEmail verifies the DB-level partial unique index
// (WHERE revoked_at IS NULL) prevents two simultaneous registrations for the same email
// from both succeeding when the application-level check races.
func TestHandleRegister_ConcurrentSameEmail(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	srv := newTestServer(t)
	const email = "race@example.com"
	const n = 5

	codes := make([]int, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			body, _ := json.Marshal(map[string]string{"email": email})
			r := httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handlerForServer(t, srv).ServeHTTP(w, r)
			codes[i] = w.Code
		}(i)
	}
	wg.Wait()

	created := 0
	for i, code := range codes {
		switch code {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			// Expected: app-level pre-check or DB partial-index race loser.
		default:
			t.Errorf("goroutine %d: status = %d, want 201 or 409; all codes: %v", i, code, codes)
		}
	}
	if created != 1 {
		t.Errorf("expected exactly 1 successful registration, got %d; all codes: %v", created, codes)
	}
}
