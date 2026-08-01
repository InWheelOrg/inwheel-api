/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS_NoAllowedOrigin_ReturnsNextUnchanged(t *testing.T) {
	t.Parallel()
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	handler := CORS("")(next)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("next was not called")
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestCORS_NoOriginHeader_PassesThrough(t *testing.T) {
	t.Parallel()
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	handler := CORS("https://example.com")(next)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("next was not called")
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestCORS_MatchingOrigin_SetsHeadersAndCallsNext(t *testing.T) {
	t.Parallel()
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	handler := CORS("https://example.com")(next)
	r := httptest.NewRequest(http.MethodPatch, "/v1/places/1/accessibility", nil)
	r.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("next was not called")
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://example.com")
	}
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want %q", got, "Origin")
	}
}

func TestCORS_NonMatchingOrigin_NoHeadersButPassesThrough(t *testing.T) {
	t.Parallel()
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	handler := CORS("https://example.com")(next)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("next was not called")
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
	}
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want %q", got, "Origin")
	}
}

func TestCORS_PreflightNonMatchingOrigin_PassesThrough(t *testing.T) {
	t.Parallel()
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	handler := CORS("https://example.com")(next)
	r := httptest.NewRequest(http.MethodOptions, "/v1/places/1/accessibility", nil)
	r.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("next was not called for a preflight request from a non-matching origin")
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestCORS_PreflightMatchingOrigin_ShortCircuits(t *testing.T) {
	t.Parallel()
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	handler := CORS("https://example.com")(next)
	r := httptest.NewRequest(http.MethodOptions, "/v1/places/1/accessibility", nil)
	r.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if called {
		t.Fatal("next was called for a preflight request")
	}
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PATCH, DELETE" {
		t.Errorf("Access-Control-Allow-Methods = %q, want %q", got, "GET, POST, PATCH, DELETE")
	}
	if got := w.Header().Get("Access-Control-Allow-Headers"); got != "X-API-Key, Content-Type" {
		t.Errorf("Access-Control-Allow-Headers = %q, want %q", got, "X-API-Key, Content-Type")
	}
}
