/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJsonResponse(t *testing.T) {
	tests := []struct {
		name       string
		data       any
		status     int
		wantStatus int
		wantBody   string
		wantHeader string
	}{
		{
			name:       "valid data",
			data:       map[string]string{"status": "ok"},
			status:     http.StatusCreated,
			wantStatus: http.StatusCreated,
			wantBody:   `{"status":"ok"}`,
			wantHeader: "application/json",
		},
		{
			name:       "unmarshalable data",
			data:       map[string]any{"fn": func() {}},
			status:     http.StatusOK,
			wantStatus: http.StatusInternalServerError,
			wantBody:   "Internal server error\n",
		},
		{
			name:       "html escaping",
			data:       map[string]string{"msg": "<b>hello</b>"},
			status:     http.StatusOK,
			wantStatus: http.StatusOK,
			wantBody:   `{"msg":"\u003cb\u003ehello\u003c/b\u003e"}`,
			wantHeader: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			jsonResponse(w, tt.data, tt.status)

			if w.Code != tt.wantStatus {
				t.Errorf("Status Code = %d, want %d", w.Code, tt.wantStatus)
			}
			if tt.wantHeader != "" && w.Header().Get("Content-Type") != tt.wantHeader {
				t.Errorf("Header = %q, want %q", w.Header().Get("Content-Type"), tt.wantHeader)
			}
			if w.Body.String() != tt.wantBody {
				t.Errorf("Body = %q, want %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestParseCoord(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		coordType  string
		wantVal    float64
		wantOk     bool
		wantStatus int
		wantBody   string
	}{
		{
			name:       "valid coordinate",
			input:      "13.405",
			coordType:  "longitude",
			wantVal:    13.405,
			wantOk:     true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "high precision",
			input:      "13.405123456789",
			coordType:  "longitude",
			wantVal:    13.405123456789,
			wantOk:     true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "zero value",
			input:      "0",
			coordType:  "radius",
			wantVal:    0,
			wantOk:     true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid coordinate",
			input:      "invalid",
			coordType:  "latitude",
			wantVal:    0,
			wantOk:     false,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Invalid latitude\n",
		},
		{
			name:       "empty string",
			input:      "",
			coordType:  "radius",
			wantVal:    0,
			wantOk:     false,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Invalid radius\n",
		},
		{
			name:       "whitespace",
			input:      " 13.405 ",
			coordType:  "longitude",
			wantVal:    0,
			wantOk:     false,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Invalid longitude\n",
		},
		{
			name:       "NaN input",
			input:      "NaN",
			coordType:  "longitude",
			wantVal:    0,
			wantOk:     false,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Invalid longitude\n",
		},
		{
			name:       "Inf input",
			input:      "Inf",
			coordType:  "latitude",
			wantVal:    0,
			wantOk:     false,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Invalid latitude\n",
		},
		{
			name:       "Injection attempt",
			input:      "13.4; DROP TABLE places",
			coordType:  "latitude",
			wantVal:    0,
			wantOk:     false,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Invalid latitude\n",
		},
		{
			name:       "Overflow input",
			input:      "1e309",
			coordType:  "radius",
			wantVal:    0,
			wantOk:     false,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Invalid radius\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			val, ok := parseCoord(w, tt.input, tt.coordType)

			if ok != tt.wantOk {
				t.Errorf("parseCoord() ok = %v, want %v", ok, tt.wantOk)
			}
			if val != tt.wantVal {
				t.Errorf("parseCoord() val = %f, want %f", val, tt.wantVal)
			}
			if w.Code != tt.wantStatus {
				t.Errorf("Status Code = %d, want %d", w.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && w.Body.String() != tt.wantBody {
				t.Errorf("Body = %q, want %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	const key = "INWHEEL_TEST_VAR"
	const fallback = "default_value"

	tests := []struct {
		name     string
		setup    func(t *testing.T)
		key      string
		fallback string
		want     string
	}{
		{
			name: "returns value when set",
			setup: func(t *testing.T) {
				t.Setenv(key, "real_value")
			},
			key:      key,
			fallback: fallback,
			want:     "real_value",
		},
		{
			name:     "returns fallback when not set",
			key:      "TOTALLY_MISSING_VAR",
			fallback: fallback,
			want:     fallback,
		},
		{
			name: "returns empty string when set but empty",
			setup: func(t *testing.T) {
				t.Setenv(key, "")
			},
			key:      key,
			fallback: fallback,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}
			got := getEnv(tt.key, tt.fallback)
			if got != tt.want {
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
	if w.Body.String() != `{"status":"ok"}` {
		t.Errorf("body = %q, want {\"status\":\"ok\"}", w.Body.String())
	}
}

func TestHandlePostPlace_InvalidJSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/places", bytes.NewBufferString("{bad json"))
	w := httptest.NewRecorder()
	(&Server{}).handlePostPlace(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandlePatchAccessibility_InvalidJSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodPatch, "/places/some-id/accessibility", bytes.NewBufferString("{bad json"))
	w := httptest.NewRecorder()
	(&Server{}).handlePatchAccessibility(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleGetPlaces_InvalidProximityCoord(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/places?lng=not-a-number&lat=52.5&radius=500", nil)
	w := httptest.NewRecorder()
	(&Server{}).handleGetPlaces(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleGetPlaces_InvalidBoundingBoxCoord(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/places?min_lng=bad&min_lat=52.0&max_lng=14.0&max_lat=53.0", nil)
	w := httptest.NewRecorder()
	(&Server{}).handleGetPlaces(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}
