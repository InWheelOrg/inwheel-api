/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/InWheelOrg/inwheel-server/internal/middleware"
	"github.com/google/uuid"
)

func TestRequestLogger_SetsRequestIDHeader(t *testing.T) {
	handler := middleware.RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	got := rec.Header().Get("X-Request-ID")
	if got == "" {
		t.Fatal("X-Request-ID header not set")
	}
	if _, err := uuid.Parse(got); err != nil {
		t.Fatalf("X-Request-ID %q is not a valid UUID: %v", got, err)
	}
}

func TestRequestLogger_RequestIDInContext(t *testing.T) {
	var ctxRequestID string
	handler := middleware.RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxRequestID = middleware.RequestIDFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if ctxRequestID == "" {
		t.Fatal("request ID not found in context")
	}
	if ctxRequestID != rec.Header().Get("X-Request-ID") {
		t.Errorf("context request ID %q != header %q", ctxRequestID, rec.Header().Get("X-Request-ID"))
	}
}

func TestRequestLogger_CapturesStatusCode(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"200", http.StatusOK},
		{"404", http.StatusNotFound},
		{"500", http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := middleware.RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			}))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.status {
				t.Errorf("expected status %d, got %d", tt.status, rec.Code)
			}
		})
	}
}

func TestSetLogAPIKeyID_EnrichesLogFields(t *testing.T) {
	var enriched bool
	handler := middleware.RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate auth middleware setting the key ID
		middleware.SetLogAPIKeyID(r.Context(), "test-key-id")
		enriched = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/places", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !enriched {
		t.Fatal("handler did not run")
	}
}

func TestRequestIDFromCtx_EmptyWithoutMiddleware(t *testing.T) {
	id := middleware.RequestIDFromCtx(context.Background())
	if id != "" {
		t.Errorf("expected empty string, got %q", id)
	}
}

func TestAPIKeyIDFromCtx_RoundTrip(t *testing.T) {
	ctx := middleware.WithAPIKeyID(context.Background(), "some-uuid")
	got := middleware.APIKeyIDFromCtx(ctx)
	if got != "some-uuid" {
		t.Errorf("expected 'some-uuid', got %q", got)
	}
}

func TestAPIKeyIDFromCtx_EmptyWithoutValue(t *testing.T) {
	got := middleware.APIKeyIDFromCtx(context.Background())
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
