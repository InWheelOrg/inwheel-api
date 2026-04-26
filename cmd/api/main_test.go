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

const validUUID = "11111111-1111-1111-1111-111111111111"

func TestHandlePatchAccessibility_InvalidJSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodPatch, "/places/"+validUUID+"/accessibility", bytes.NewBufferString("{bad json"))
	r.SetPathValue("id", validUUID)
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

func TestHandlePostPlace_400OnLatOutOfBounds(t *testing.T) {
	body := `{"name":"X","lat":91,"lng":13.4,"category":"cafe"}`
	r := httptest.NewRequest(http.MethodPost, "/places", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	(&Server{}).handlePostPlace(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	msg, fields := decodeValidationError(t, w.Body.Bytes())
	if msg != "validation failed" {
		t.Errorf("error = %q, want validation failed", msg)
	}
	if len(fields) == 0 || fields[0]["field"] != "lat" {
		t.Errorf("expected field error on lat, got %+v", fields)
	}
}

func TestHandlePostPlace_400OnMissingName(t *testing.T) {
	body := `{"lat":52.5,"lng":13.4,"category":"cafe"}`
	r := httptest.NewRequest(http.MethodPost, "/places", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	(&Server{}).handlePostPlace(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	_, fields := decodeValidationError(t, w.Body.Bytes())
	if !hasField(fields, "name") {
		t.Errorf("expected field error on name, got %+v", fields)
	}
}

func TestHandlePostPlace_400OnInvalidCategory(t *testing.T) {
	body := `{"name":"X","lat":52.5,"lng":13.4,"category":"spaceship"}`
	r := httptest.NewRequest(http.MethodPost, "/places", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	(&Server{}).handlePostPlace(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	_, fields := decodeValidationError(t, w.Body.Bytes())
	if !hasField(fields, "category") {
		t.Errorf("expected field error on category, got %+v", fields)
	}
}

func TestHandleGetPlace_400OnInvalidUUID(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/places/not-a-uuid", nil)
	r.SetPathValue("id", "not-a-uuid")
	w := httptest.NewRecorder()
	(&Server{}).handleGetPlace(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	_, fields := decodeValidationError(t, w.Body.Bytes())
	if !hasField(fields, "id") {
		t.Errorf("expected field error on id, got %+v", fields)
	}
}

func TestHandleGetPlaces_400OnRadiusNonPositive(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/places?lng=13.4&lat=52.5&radius=-5", nil)
	w := httptest.NewRecorder()
	(&Server{}).handleGetPlaces(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	_, fields := decodeValidationError(t, w.Body.Bytes())
	if !hasField(fields, "radius") {
		t.Errorf("expected field error on radius, got %+v", fields)
	}
}

func TestHandleGetPlaces_400OnPartialBoundingBox(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/places?min_lng=13.0&min_lat=52.0&max_lng=14.0", nil)
	w := httptest.NewRecorder()
	(&Server{}).handleGetPlaces(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandlePatchAccessibility_400OnInvalidStatus(t *testing.T) {
	body := `{"overall_status":"not-a-status"}`
	r := httptest.NewRequest(http.MethodPatch, "/places/"+validUUID+"/accessibility", bytes.NewBufferString(body))
	r.SetPathValue("id", validUUID)
	w := httptest.NewRecorder()
	(&Server{}).handlePatchAccessibility(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	_, fields := decodeValidationError(t, w.Body.Bytes())
	if !hasField(fields, "overall_status") {
		t.Errorf("expected field error on overall_status, got %+v", fields)
	}
}

func hasField(fields []map[string]string, field string) bool {
	for _, f := range fields {
		if f["field"] == field {
			return true
		}
	}
	return false
}
