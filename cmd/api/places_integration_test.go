//go:build integration

/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/InWheelOrg/inwheel-api/internal/pagination"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// decodePageResponse decodes the paginated GET /places envelope.
func decodePageResponse(t *testing.T, body []byte) ([]models.Place, string) {
	t.Helper()
	var page struct {
		Data       []models.Place `json:"data"`
		NextCursor string         `json:"next_cursor"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("decode page response: %v\nbody: %s", err, body)
	}
	return page.Data, page.NextCursor
}

func TestHandleGetPlaces_DefaultPagination(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	// Insert 3 places with distinct updated_at so ordering is deterministic.
	for i := 0; i < 3; i++ {
		p := models.Place{
			Name:      fmt.Sprintf("Place %d", i),
			Lat:       52.5,
			Lng:       13.4,
			Category:  models.CategoryCafe,
			Source:    "test",
			UpdatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		testDB.Create(&p)
	}

	r := httptest.NewRequest(http.MethodGet, "/v1/places?limit=2", nil)
	w := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	places, nextCursor := decodePageResponse(t, w.Body.Bytes())

	if len(places) != 2 {
		t.Errorf("got %d places, want 2", len(places))
	}
	if nextCursor == "" {
		t.Error("expected next_cursor to be set when more results exist")
	}
}

func TestHandleGetPlaces_LastPageHasNoCursor(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	for i := 0; i < 2; i++ {
		p := models.Place{
			Name:     fmt.Sprintf("Place %d", i),
			Lat:      52.5,
			Lng:      13.4,
			Category: models.CategoryCafe,
			Source:   "test",
		}
		testDB.Create(&p)
	}

	r := httptest.NewRequest(http.MethodGet, "/v1/places?limit=10", nil)
	w := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	places, nextCursor := decodePageResponse(t, w.Body.Bytes())

	if len(places) != 2 {
		t.Errorf("got %d places, want 2", len(places))
	}
	if nextCursor != "" {
		t.Errorf("expected no next_cursor on last page, got %q", nextCursor)
	}
}

func TestHandleGetPlaces_CursorAdvancesPage(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	base := time.Now().UTC().Truncate(time.Microsecond)
	var allIDs []string
	for i := 0; i < 5; i++ {
		p := models.Place{
			Name:      fmt.Sprintf("Place %d", i),
			Lat:       52.5,
			Lng:       13.4,
			Category:  models.CategoryCafe,
			Source:    "test",
			UpdatedAt: base.Add(time.Duration(i) * time.Second),
		}
		testDB.Create(&p)
		allIDs = append(allIDs, p.ID)
	}

	// First page: expect 3 results and a cursor.
	r1 := httptest.NewRequest(http.MethodGet, "/v1/places?limit=3", nil)
	w1 := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w1, r1)

	if w1.Code != http.StatusOK {
		t.Fatalf("page 1 status = %d; body: %s", w1.Code, w1.Body.String())
	}
	page1, cursor := decodePageResponse(t, w1.Body.Bytes())
	if len(page1) != 3 {
		t.Fatalf("page 1: got %d places, want 3", len(page1))
	}
	if cursor == "" {
		t.Fatal("expected cursor after page 1")
	}

	// Second page using the cursor.
	r2 := httptest.NewRequest(http.MethodGet, "/v1/places?limit=3&cursor="+cursor, nil)
	w2 := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w2, r2)

	if w2.Code != http.StatusOK {
		t.Fatalf("page 2 status = %d; body: %s", w2.Code, w2.Body.String())
	}
	page2, cursor2 := decodePageResponse(t, w2.Body.Bytes())
	if len(page2) != 2 {
		t.Fatalf("page 2: got %d places, want 2", len(page2))
	}
	if cursor2 != "" {
		t.Errorf("expected no cursor after last page, got %q", cursor2)
	}

	// No overlap between pages.
	page1IDs := make(map[string]bool)
	for _, p := range page1 {
		page1IDs[p.ID] = true
	}
	for _, p := range page2 {
		if page1IDs[p.ID] {
			t.Errorf("place %q appeared in both page 1 and page 2", p.ID)
		}
	}
}

func TestHandleGetPlaces_DefaultLimitIs20(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	for i := 0; i < 25; i++ {
		p := models.Place{
			Name:     fmt.Sprintf("Place %d", i),
			Lat:      52.5,
			Lng:      13.4,
			Category: models.CategoryCafe,
			Source:   "test",
		}
		testDB.Create(&p)
	}

	r := httptest.NewRequest(http.MethodGet, "/v1/places", nil)
	w := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	places, _ := decodePageResponse(t, w.Body.Bytes())
	if len(places) != 20 {
		t.Errorf("got %d places with no limit param, want 20 (default)", len(places))
	}
}

func TestHandleGetPlaces_InvalidCursorReturns400(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/places?cursor=not-valid-base64!!!!", nil)
	w := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleGetPlaces_InvalidLimitReturns400(t *testing.T) {
	for _, bad := range []string{"0", "101", "abc", "-5"} {
		t.Run(bad, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/v1/places?limit="+bad, nil)
			w := httptest.NewRecorder()
			handlerForServer(t, newTestServer(t)).ServeHTTP(w, r)

			if w.Code != http.StatusBadRequest {
				t.Errorf("limit=%s: status = %d, want 400", bad, w.Code)
			}
		})
	}
}

func TestHandleGetPlaces_ProximityPaginated(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	// 5 places in Berlin, 1 far away.
	berlin := []models.Place{}
	for i := 0; i < 5; i++ {
		p := models.Place{
			Name:     fmt.Sprintf("Berlin Place %d", i),
			Lat:      52.52 + float64(i)*0.001,
			Lng:      13.405,
			Category: models.CategoryCafe,
			Source:   "test",
		}
		testDB.Create(&p)
		berlin = append(berlin, p)
	}
	london := models.Place{Name: "London", Lat: 51.507, Lng: -0.127, Category: models.CategoryCafe, Source: "test"}
	testDB.Create(&london)

	// First page of proximity query.
	r := httptest.NewRequest(http.MethodGet, "/v1/places?lng=13.405&lat=52.52&radius=5000&limit=3", nil)
	w := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	places, cursor := decodePageResponse(t, w.Body.Bytes())
	if len(places) != 3 {
		t.Errorf("got %d places, want 3", len(places))
	}
	if cursor == "" {
		t.Error("expected cursor with more results remaining")
	}

	// Second page.
	r2 := httptest.NewRequest(http.MethodGet, "/v1/places?lng=13.405&lat=52.52&radius=5000&limit=3&cursor="+cursor, nil)
	w2 := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w2, r2)

	if w2.Code != http.StatusOK {
		t.Fatalf("page 2 status = %d; body: %s", w2.Code, w2.Body.String())
	}
	places2, _ := decodePageResponse(t, w2.Body.Bytes())
	if len(places2) != 2 {
		t.Errorf("page 2: got %d places, want 2", len(places2))
	}
	// London should not appear in either page (outside radius).
	for _, p := range append(places, places2...) {
		if p.Name == "London" {
			t.Error("London should be outside the 5km radius")
		}
	}
}

func TestHandleGetPlaces_BoundingBoxPaginated(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	for i := 0; i < 4; i++ {
		p := models.Place{
			Name:     fmt.Sprintf("Berlin %d", i),
			Lat:      52.5 + float64(i)*0.01,
			Lng:      13.4,
			Category: models.CategoryCafe,
			Source:   "test",
		}
		testDB.Create(&p)
	}
	london := models.Place{Name: "London", Lat: 51.507, Lng: -0.127, Category: models.CategoryCafe, Source: "test"}
	testDB.Create(&london)

	r := httptest.NewRequest(http.MethodGet, "/v1/places?min_lng=10.0&min_lat=50.0&max_lng=15.0&max_lat=55.0&limit=3", nil)
	w := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	places, cursor := decodePageResponse(t, w.Body.Bytes())
	if len(places) != 3 {
		t.Errorf("got %d places, want 3", len(places))
	}
	if cursor == "" {
		t.Error("expected cursor with 1 more result")
	}

	r2 := httptest.NewRequest(http.MethodGet, "/v1/places?min_lng=10.0&min_lat=50.0&max_lng=15.0&max_lat=55.0&limit=3&cursor="+cursor, nil)
	w2 := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w2, r2)

	if w2.Code != http.StatusOK {
		t.Fatalf("page 2 status = %d; body: %s", w2.Code, w2.Body.String())
	}
	places2, cursor2 := decodePageResponse(t, w2.Body.Bytes())
	if len(places2) != 1 {
		t.Errorf("page 2: got %d places, want 1", len(places2))
	}
	if cursor2 != "" {
		t.Errorf("expected no cursor on last page, got %q", cursor2)
	}
	for _, p := range append(places, places2...) {
		if p.Name == "London" {
			t.Error("London is outside bounding box and should not appear")
		}
	}
}

func TestHandleGetPlaces_PreloadsAccessibility(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	p := models.Place{
		Name:     "Accessible Cafe",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryCafe,
		Source:   "test",
		Accessibility: &models.AccessibilityProfile{
			OverallStatus: models.StatusAccessible,
		},
	}
	testDB.Create(&p)

	r := httptest.NewRequest(http.MethodGet, "/v1/places", nil)
	w := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	places, _ := decodePageResponse(t, w.Body.Bytes())
	if len(places) != 1 {
		t.Fatalf("got %d places, want 1", len(places))
	}
	if places[0].Accessibility == nil {
		t.Error("expected accessibility to be preloaded in GET /places response")
	}
}

func TestHandleGetPlaces_EmptyDB(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	r := httptest.NewRequest(http.MethodGet, "/v1/places", nil)
	w := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	places, cursor := decodePageResponse(t, w.Body.Bytes())
	if len(places) != 0 {
		t.Errorf("got %d places, want 0", len(places))
	}
	if cursor != "" {
		t.Errorf("expected no cursor for empty result, got %q", cursor)
	}
}

// Verify the pagination.Encode/Decode round-trip survives URL query encoding.
func TestHandleGetPlaces_CursorURLSafe(t *testing.T) {
	ts := time.Now().UTC()
	id := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	encoded := pagination.Encode(ts, id)

	// The cursor should survive being used as a query param.
	q := "cursor=" + encoded
	parsed, err := http.NewRequest(http.MethodGet, "/v1/places?"+q, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	got := parsed.URL.Query().Get("cursor")
	if got != encoded {
		t.Errorf("cursor mangled in URL: got %q, want %q", got, encoded)
	}
}
