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
	r := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleRegister(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("registerKey: status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("registerKey: decode response: %v", err)
	}
	return resp["key"]
}

// --- Registration tests ---

func TestHandleRegister_Success(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	body, _ := json.Marshal(map[string]string{"email": "user@example.com"})
	r := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newTestServer().handleRegister(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(resp["key"], "iwk_") {
		t.Errorf("key %q does not start with iwk_", resp["key"])
	}
	if len(resp["key"]) != 68 { // "iwk_" (4) + 64 hex chars
		t.Errorf("key length = %d, want 68", len(resp["key"]))
	}
	if resp["note"] == "" {
		t.Error("expected non-empty note in response")
	}
}

func TestHandleRegister_DuplicateActiveKey(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	srv := newTestServer()
	registerKey(t, srv, "dup@example.com")

	body, _ := json.Marshal(map[string]string{"email": "dup@example.com"})
	r := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleRegister(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleRegister_InvalidEmail(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	body, _ := json.Marshal(map[string]string{"email": "not-an-email"})
	r := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newTestServer().handleRegister(w, r)

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
	srv := &Server{
		db:         testDB,
		engine:     newTestServer().engine,
		regLimiter: middleware.NewRateLimiter(rate.Every(24*time.Hour), 2),
		keyLimiter: middleware.NewRateLimiter(rate.Every(time.Millisecond), 1000),
	}

	emails := []string{"a@example.com", "b@example.com", "c@example.com"}
	for i, email := range emails {
		body, _ := json.Marshal(map[string]string{"email": email})
		r := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.handleRegister(w, r)

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

	srv := newTestServer()
	rawKey := registerKey(t, srv, "writer@example.com")

	body, _ := json.Marshal(models.Place{
		Name:     "Test Place",
		Lat:      60.1,
		Lng:      24.9,
		Category: models.CategoryCafe,
		Rank:     models.RankEstablishment,
	})
	r := httptest.NewRequest(http.MethodPost, "/places", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	middleware.RequireAPIKey(testDB, srv.keyLimiter, srv.handlePostPlace)(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
}

func TestHandlePostPlace_MissingAuth(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	srv := newTestServer()
	body, _ := json.Marshal(models.Place{Name: "x", Lat: 60.1, Lng: 24.9, Category: models.CategoryCafe, Rank: models.RankEstablishment})
	r := httptest.NewRequest(http.MethodPost, "/places", bytes.NewReader(body))
	w := httptest.NewRecorder()

	middleware.RequireAPIKey(testDB, srv.keyLimiter, srv.handlePostPlace)(w, r)

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

	srv := newTestServer()
	body, _ := json.Marshal(models.Place{Name: "x", Lat: 60.1, Lng: 24.9, Category: models.CategoryCafe, Rank: models.RankEstablishment})
	r := httptest.NewRequest(http.MethodPost, "/places", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer iwk_notavalidkey000000000000000000000000000000000000000000000000000")
	w := httptest.NewRecorder()

	middleware.RequireAPIKey(testDB, srv.keyLimiter, srv.handlePostPlace)(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", w.Code, w.Body.String())
	}
}

func TestHandlePostPlace_RevokedKey(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	srv := newTestServer()
	rawKey := registerKey(t, srv, "revoked@example.com")

	// Revoke the key directly in the DB.
	now := time.Now()
	testDB.Model(&models.APIKey{}).Where("email = ?", "revoked@example.com").Update("revoked_at", now)

	body, _ := json.Marshal(models.Place{Name: "x", Lat: 60.1, Lng: 24.9, Category: models.CategoryCafe, Rank: models.RankEstablishment})
	r := httptest.NewRequest(http.MethodPost, "/places", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	middleware.RequireAPIKey(testDB, srv.keyLimiter, srv.handlePostPlace)(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", w.Code, w.Body.String())
	}
}

func TestHandlePatchAccessibility_WithValidKey(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	srv := newTestServer()

	// Create a place first (directly in DB).
	place := models.Place{Name: "A Place", Lat: 60.1, Lng: 24.9, Category: models.CategoryCafe, Rank: models.RankEstablishment}
	testDB.Create(&place)

	rawKey := registerKey(t, srv, "patcher@example.com")

	profile := models.AccessibilityProfile{OverallStatus: models.StatusAccessible}
	body, _ := json.Marshal(profile)
	r := httptest.NewRequest(http.MethodPatch, "/places/"+place.ID+"/accessibility", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+rawKey)
	r.SetPathValue("id", place.ID)
	w := httptest.NewRecorder()

	middleware.RequireAPIKey(testDB, srv.keyLimiter, srv.handlePatchAccessibility)(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestHandlePatchAccessibility_MissingAuth(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	srv := newTestServer()
	r := httptest.NewRequest(http.MethodPatch, "/places/some-id/accessibility", strings.NewReader(`{"overall_status":"accessible"}`))
	w := httptest.NewRecorder()

	middleware.RequireAPIKey(testDB, srv.keyLimiter, srv.handlePatchAccessibility)(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", w.Code, w.Body.String())
	}
}
