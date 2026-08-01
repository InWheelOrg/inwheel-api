/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/InWheelOrg/inwheel-api/internal/middleware"
	"golang.org/x/time/rate"
)

func TestBuildV1Handler_UnverifiedAPIKeyHeaderDoesNotBypassReadLimit(t *testing.T) {
	srv := &Server{
		readLimiter: middleware.NewRateLimiter(t.Context(), rate.Every(time.Hour), 1),
	}
	handler, err := buildV1Handler(srv, "")
	if err != nil {
		t.Fatalf("buildV1Handler: %v", err)
	}

	newReq := func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/v1/places?lng=1", nil)
		r.RemoteAddr = "1.2.3.4:9999"
		r.Header.Set("X-API-Key", "garbage")
		return r
	}

	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, newReq())
	if w1.Code != http.StatusBadRequest {
		t.Fatalf("first request status = %d, want %d (invalid proximity params, no DB touch)", w1.Code, http.StatusBadRequest)
	}

	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, newReq())
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want %d (an unverified X-API-Key header must not bypass the read limiter)", w2.Code, http.StatusTooManyRequests)
	}
}
