/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/InWheelOrg/inwheel-server/internal/middleware"
	"github.com/getkin/kin-openapi/openapi3filter"
	"golang.org/x/time/rate"
)

func newErrorHandlerServer(t *testing.T) *Server {
	t.Helper()
	ctx := t.Context()
	return &Server{
		keyLimiter: middleware.NewRateLimiter(ctx, rate.Every(1), 60),
	}
}

func TestValidationErrorHandler_FormatsFieldErrors(t *testing.T) {
	srv := newErrorHandlerServer(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	reqErr := &openapi3filter.RequestError{
		Input:  nil,
		Err:    openapi3filter.ErrInvalidRequired,
		Reason: "missing required parameter 'lat'",
	}

	srv.validationErrorHandler(w, r, reqErr)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp struct {
		Error  string `json:"error"`
		Fields []struct {
			Field  string `json:"field"`
			Reason string `json:"reason"`
		} `json:"fields"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != "validation failed" {
		t.Errorf("error = %q, want 'validation failed'", resp.Error)
	}
	if len(resp.Fields) == 0 {
		t.Error("expected at least one field error")
	}
}

func TestValidationErrorHandler_SecurityError_Unauthorized(t *testing.T) {
	srv := newErrorHandlerServer(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	secErr := &openapi3filter.SecurityRequirementsError{
		Errors: []error{errUnauthorized},
	}

	srv.validationErrorHandler(w, r, secErr)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"] != "unauthorized" {
		t.Errorf("error = %q, want 'unauthorized'", resp["error"])
	}
}

func TestValidationErrorHandler_SecurityError_RateLimited(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv := &Server{
		keyLimiter: middleware.NewRateLimiter(ctx, rate.Every(1), 60),
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	secErr := &openapi3filter.SecurityRequirementsError{
		Errors: []error{errRateLimited},
	}

	srv.validationErrorHandler(w, r, secErr)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
	if ra := w.Header().Get("Retry-After"); ra == "" {
		t.Error("expected Retry-After header on 429")
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"] != "rate limit exceeded" {
		t.Errorf("error = %q, want 'rate limit exceeded'", resp["error"])
	}
}
