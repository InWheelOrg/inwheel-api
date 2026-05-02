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
	"time"

	"github.com/InWheelOrg/inwheel-server/internal/a11y"
	apiv1 "github.com/InWheelOrg/inwheel-server/internal/api/v1"
	"github.com/InWheelOrg/inwheel-server/internal/middleware"
	"github.com/getkin/kin-openapi/openapi3filter"
	nethttp_middleware "github.com/oapi-codegen/nethttp-middleware"
	"golang.org/x/time/rate"
)

// handlerForServer builds the full HTTP handler stack (mux + auth + spec validator)
// from a given Server. Used by both unit and integration tests.
func handlerForServer(t *testing.T, srv *Server) http.Handler {
	t.Helper()

	swagger, err := apiv1.GetSpec()
	if err != nil {
		t.Fatalf("load swagger: %v", err)
	}

	v1Mux := http.NewServeMux()
	strictHandler := apiv1.NewStrictHandlerWithOptions(srv, []apiv1.StrictMiddlewareFunc{injectRequest()}, apiv1.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  srv.validationErrorHandler,
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, _ error) {
			writeJSON(w, map[string]string{"error": "internal server error"}, http.StatusInternalServerError)
		},
	})
	apiv1.HandlerWithOptions(strictHandler, apiv1.StdHTTPServerOptions{
		BaseURL:          "/v1",
		BaseRouter:       v1Mux,
		ErrorHandlerFunc: srv.validationErrorHandler,
		Middlewares:      []apiv1.MiddlewareFunc{bodySizeLimiter(1 << 20)},
	})

	v1Handler := nethttp_middleware.OapiRequestValidatorWithOptions(swagger, &nethttp_middleware.Options{
		SilenceServersWarning: true,
		Options: openapi3filter.Options{
			AuthenticationFunc: srv.authenticate,
		},
		ErrorHandlerWithOpts: func(_ context.Context, err error, w http.ResponseWriter, r *http.Request, _ nethttp_middleware.ErrorHandlerOpts) {
			srv.validationErrorHandler(w, r, err)
		},
	})(v1Mux)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", srv.handleHealthz)
	mux.HandleFunc("GET /readyz", srv.handleReadyz)
	mux.HandleFunc("GET /openapi.yaml", srv.handleOpenAPISpec)
	mux.Handle("/v1/", v1Handler)
	return mux
}

// handlerNoAuth builds the same stack as handlerForServer but skips auth.
// Use for handler-focused integration tests that aren't exercising auth.
func handlerNoAuth(t *testing.T, srv *Server) http.Handler {
	t.Helper()

	swagger, err := apiv1.GetSpec()
	if err != nil {
		t.Fatalf("load swagger: %v", err)
	}

	v1Mux := http.NewServeMux()
	strictHandler := apiv1.NewStrictHandlerWithOptions(srv, []apiv1.StrictMiddlewareFunc{injectRequest()}, apiv1.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  srv.validationErrorHandler,
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, _ error) {
			writeJSON(w, map[string]string{"error": "internal server error"}, http.StatusInternalServerError)
		},
	})
	apiv1.HandlerWithOptions(strictHandler, apiv1.StdHTTPServerOptions{
		BaseURL:          "/v1",
		BaseRouter:       v1Mux,
		ErrorHandlerFunc: srv.validationErrorHandler,
		Middlewares:      []apiv1.MiddlewareFunc{bodySizeLimiter(1 << 20)},
	})

	v1Handler := nethttp_middleware.OapiRequestValidatorWithOptions(swagger, &nethttp_middleware.Options{
		SilenceServersWarning: true,
		Options: openapi3filter.Options{
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		},
		ErrorHandlerWithOpts: func(_ context.Context, err error, w http.ResponseWriter, r *http.Request, _ nethttp_middleware.ErrorHandlerOpts) {
			srv.validationErrorHandler(w, r, err)
		},
	})(v1Mux)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", srv.handleHealthz)
	mux.HandleFunc("GET /readyz", srv.handleReadyz)
	mux.HandleFunc("GET /openapi.yaml", srv.handleOpenAPISpec)
	mux.Handle("/v1/", v1Handler)
	return mux
}

// newValidationHandler creates a full handler with a nil DB for validation-only tests.
// Only use for requests that fail before hitting the database.
func newValidationHandler(t *testing.T) http.Handler {
	t.Helper()
	ctx := t.Context()
	return handlerForServer(t, &Server{
		engine:     &a11y.Engine{},
		regLimiter: middleware.NewRateLimiter(ctx, rate.Every(time.Millisecond), 1000),
		keyLimiter: middleware.NewRateLimiter(ctx, rate.Every(time.Millisecond), 1000),
	})
}

// ── Infrastructure ────────────────────────────────────────────────────────────

func TestWriteJSON(t *testing.T) {
	tests := []struct {
		name       string
		data       any
		status     int
		wantStatus int
		wantBody   string
	}{
		{
			name:       "sets status and content-type",
			data:       map[string]string{"status": "ok"},
			status:     http.StatusCreated,
			wantStatus: http.StatusCreated,
			wantBody:   `{"status":"ok"}` + "\n",
		},
		{
			name:       "html escaping",
			data:       map[string]string{"msg": "<b>hi</b>"},
			status:     http.StatusOK,
			wantStatus: http.StatusOK,
			wantBody:   `{"msg":"<b>hi</b>"}` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeJSON(w, tt.data, tt.status)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if w.Header().Get("Content-Type") != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", w.Header().Get("Content-Type"))
			}
			if w.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	const key = "INWHEEL_TEST_VAR"

	tests := []struct {
		name     string
		setup    func(t *testing.T)
		key      string
		fallback string
		want     string
	}{
		{
			name:     "returns value when set",
			setup:    func(t *testing.T) { t.Setenv(key, "real_value") },
			key:      key,
			fallback: "default",
			want:     "real_value",
		},
		{
			name:     "returns fallback when not set",
			key:      "TOTALLY_MISSING_VAR",
			fallback: "default",
			want:     "default",
		},
		{
			name:     "returns empty string when set but empty",
			setup:    func(t *testing.T) { t.Setenv(key, "") },
			key:      key,
			fallback: "default",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}
			if got := getEnv(tt.key, tt.fallback); got != tt.want {
				t.Errorf("getEnv() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHandleHealthz(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	(&Server{}).handleHealthz(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != `{"status":"ok"}`+"\n" {
		t.Errorf("body = %q", w.Body.String())
	}
}

// ── GET /v1/places validation (public, no auth required) ─────────────────────

func TestHandleGetPlaces_InvalidProximityCoord(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/places?lng=not-a-number&lat=52.5&radius=500", nil)
	w := httptest.NewRecorder()
	newValidationHandler(t).ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleGetPlaces_InvalidBoundingBoxCoord(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/places?min_lng=bad&min_lat=52.0&max_lng=14.0&max_lat=53.0", nil)
	w := httptest.NewRecorder()
	newValidationHandler(t).ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleGetPlaces_400OnRadiusNonPositive(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/places?lng=13.4&lat=52.5&radius=-5", nil)
	w := httptest.NewRecorder()
	newValidationHandler(t).ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	_, fields := decodeValidationError(t, w.Body.Bytes())
	if !hasField(fields, "radius") {
		t.Errorf("expected field error on radius, got %+v", fields)
	}
}

func TestHandleGetPlaces_400OnPartialBoundingBox(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/places?min_lng=13.0&min_lat=52.0&max_lng=14.0", nil)
	w := httptest.NewRecorder()
	newValidationHandler(t).ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// ── GET /v1/places/{id} validation (public, no auth required) ────────────────

func TestHandleGetPlace_400OnInvalidUUID(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/places/not-a-uuid", nil)
	w := httptest.NewRecorder()
	newValidationHandler(t).ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	_, fields := decodeValidationError(t, w.Body.Bytes())
	if !hasField(fields, "id") {
		t.Errorf("expected field error on id, got %+v", fields)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func decodeValidationError(t *testing.T, body []byte) (string, []map[string]string) {
	t.Helper()
	var resp struct {
		Error  string              `json:"error"`
		Fields []map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode validation error response: %v", err)
	}
	return resp.Error, resp.Fields
}

func hasField(fields []map[string]string, field string) bool {
	for _, f := range fields {
		if f["field"] == field {
			return true
		}
	}
	return false
}
