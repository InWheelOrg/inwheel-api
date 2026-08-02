//go:build integration

/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/InWheelOrg/inwheel-api/internal/a11y"
	"github.com/InWheelOrg/inwheel-api/internal/middleware"
	"github.com/InWheelOrg/inwheel-api/internal/place"
	"github.com/InWheelOrg/inwheel-api/internal/testhelpers"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
	"golang.org/x/time/rate"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

var testDB *gorm.DB

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()
	var cleanup func()
	var err error

	testDB, cleanup, err = testhelpers.StartPostgres(ctx)
	if err != nil {
		log.Fatalf("start test postgres: %v", err)
	}
	defer cleanup()

	return m.Run()
}

func truncate(t *testing.T) {
	t.Helper()
	testDB.Exec("TRUNCATE places, accessibility_profiles, api_keys, write_logs CASCADE")
}

func boolPtr(b bool) *bool { return &b }

func newTestServer(t *testing.T) *Server {
	t.Helper()
	ctx := t.Context()
	return &Server{
		db:         testDB,
		places:     place.NewRepository(testDB),
		engine:     &a11y.Engine{},
		regLimiter: middleware.NewRateLimiter(ctx, rate.Every(time.Millisecond), 1000),
		keyLimiter: middleware.NewRateLimiter(ctx, rate.Every(time.Millisecond), 1000),
	}
}

func TestHandlePostPlace_WithAccessibility(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	body, _ := json.Marshal(models.Place{
		Name:     "Test Cafe",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryCafe,
		Rank:     models.RankEstablishment,
		Source:   "test",
		Accessibility: &models.AccessibilityProfile{
			Entrance: &models.EntranceProps{IsLevel: boolPtr(true)},
		},
	})

	r := httptest.NewRequest(http.MethodPost, "/v1/places", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlerNoAuth(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var place models.Place
	testDB.Preload("Accessibility").Last(&place)

	if place.Accessibility == nil {
		t.Fatal("expected accessibility profile to be created")
	}
}

func TestHandlePostPlace_WithoutAccessibility(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	body, _ := json.Marshal(models.Place{
		Name:     "Test Cafe",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryCafe,
		Rank:     models.RankEstablishment,
		Source:   "test",
	})

	r := httptest.NewRequest(http.MethodPost, "/v1/places", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlerNoAuth(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var place models.Place
	testDB.Preload("Accessibility").Last(&place)

	if place.Accessibility != nil {
		t.Error("expected no accessibility profile when none submitted")
	}
}

func TestHandlePostPlace_InformationalFlagsAllowed(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	// narrow width sets an audit flag but must not block the write
	narrowWidth := models.LevelNo
	body, _ := json.Marshal(models.Place{
		Name:     "Narrow Cafe",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryCafe,
		Rank:     models.RankEstablishment,
		Source:   "test",
		Accessibility: &models.AccessibilityProfile{
			Entrance: &models.EntranceProps{Width: &narrowWidth},
		},
	})

	r := httptest.NewRequest(http.MethodPost, "/v1/places", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlerNoAuth(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var place models.Place
	testDB.Preload("Accessibility").Last(&place)
	if place.Accessibility == nil || place.Accessibility.Entrance == nil {
		t.Fatal("expected accessibility with entrance component")
	}
	found := false
	for _, f := range place.Accessibility.Entrance.AuditFlags {
		if f == a11y.FlagEntranceNarrowWidth {
			found = true
		}
	}
	if !found {
		t.Errorf("expected narrow width flag to be stored, got flags: %v", place.Accessibility.Entrance.AuditFlags)
	}
}

func TestHandlePatchAccessibility_PlaceNotFound(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	const nonExistentID = "00000000-0000-0000-0000-000000000000"
	body, _ := json.Marshal(models.AccessibilityProfile{Entrance: &models.EntranceProps{IsLevel: boolPtr(true)}})

	r := httptest.NewRequest(http.MethodPatch, "/v1/places/"+nonExistentID+"/accessibility", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.SetPathValue("id", nonExistentID)
	w := httptest.NewRecorder()
	handlerNoAuth(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", w.Code, w.Body.String())
	}
}

func TestHandlePatchAccessibility_CreatePath(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	place := models.Place{Name: "Test Place", Lat: 52.5, Lng: 13.4, Category: models.CategoryCafe, Rank: models.RankEstablishment, Source: "test"}
	testDB.Create(&place)

	body, _ := json.Marshal(models.AccessibilityProfile{
		Entrance: &models.EntranceProps{IsLevel: boolPtr(false)},
	})

	r := httptest.NewRequest(http.MethodPatch, "/v1/places/"+place.ID+"/accessibility", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.SetPathValue("id", place.ID)
	w := httptest.NewRecorder()
	handlerNoAuth(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var profile models.AccessibilityProfile
	testDB.Where("place_id = ?", place.ID).First(&profile)

	if profile.PlaceID != place.ID {
		t.Errorf("PlaceID = %s, want %s", profile.PlaceID, place.ID)
	}
	if profile.Entrance == nil || profile.Entrance.IsLevel == nil || *profile.Entrance.IsLevel {
		t.Error("expected Entrance.IsLevel=false")
	}
}

func TestHandlePatchAccessibility_UpdatesExistingProfile(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	place := models.Place{
		Name:     "Test Place",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryCafe,
		Rank:     models.RankEstablishment,
		Source:   "test",
		Accessibility: &models.AccessibilityProfile{
			Entrance: &models.EntranceProps{IsLevel: boolPtr(true)},
		},
	}
	testDB.Create(&place)

	body, _ := json.Marshal(models.AccessibilityProfile{
		Entrance: &models.EntranceProps{IsLevel: boolPtr(false)},
	})

	r := httptest.NewRequest(http.MethodPatch, "/v1/places/"+place.ID+"/accessibility", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.SetPathValue("id", place.ID)
	w := httptest.NewRecorder()
	handlerNoAuth(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var profile models.AccessibilityProfile
	testDB.Where("place_id = ?", place.ID).First(&profile)

	if profile.Entrance == nil || profile.Entrance.IsLevel == nil || *profile.Entrance.IsLevel {
		t.Error("expected Entrance.IsLevel=false after update")
	}
}

func TestHandleGetPlace_ReturnsPlaceWithAccessibility(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	place := models.Place{
		Name:     "Test Cafe",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryCafe,
		Rank:     models.RankEstablishment,
		Source:   "test",
		Accessibility: &models.AccessibilityProfile{
			Entrance: &models.EntranceProps{IsLevel: boolPtr(true)},
		},
	}
	testDB.Create(&place)

	r := httptest.NewRequest(http.MethodGet, "/v1/places/"+place.ID, nil)
	r.SetPathValue("id", place.ID)
	w := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var got models.Place
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Accessibility == nil {
		t.Error("expected accessibility profile in response")
	}
}

func TestHandleGetPlace_NotFound(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	const nonExistentID = "00000000-0000-0000-0000-000000000000"
	r := httptest.NewRequest(http.MethodGet, "/v1/places/"+nonExistentID, nil)
	r.SetPathValue("id", nonExistentID)
	w := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandleGetPlace_InheritsParentComponents(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	parent := models.Place{
		Name:     "Test Mall",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryMall,
		Rank:     models.RankEstablishment,
		Source:   "test",
		Accessibility: &models.AccessibilityProfile{
			Parking: &models.ParkingProps{HasDisabledSpaces: boolPtr(true)},
		},
	}
	testDB.Create(&parent)

	child := models.Place{
		Name:     "Test Shop",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryShop,
		Rank:     models.RankEstablishment,
		Source:   "test",
		ParentID: &parent.ID,
	}
	testDB.Create(&child)

	r := httptest.NewRequest(http.MethodGet, "/v1/places/"+child.ID, nil)
	r.SetPathValue("id", child.ID)
	w := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var got models.Place
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Accessibility == nil {
		t.Fatal("expected accessibility profile in response")
	}
	if got.Accessibility.Parking == nil {
		t.Fatal("expected inherited parking in effective profile")
	}
	if !got.Accessibility.Parking.IsInherited {
		t.Error("parking should be marked is_inherited=true")
	}
	if got.Accessibility.Parking.SourceID != parent.ID {
		t.Errorf("source_id = %q, want %q", got.Accessibility.Parking.SourceID, parent.ID)
	}
}

func TestHandleGetPlace_ChildOverridesParentComponent(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	parent := models.Place{
		Name:     "Test Mall",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryMall,
		Rank:     models.RankEstablishment,
		Source:   "test",
		Accessibility: &models.AccessibilityProfile{
			Entrance: &models.EntranceProps{IsLevel: boolPtr(true)},
		},
	}
	testDB.Create(&parent)

	child := models.Place{
		Name:     "Test Shop",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryShop,
		Rank:     models.RankEstablishment,
		Source:   "test",
		ParentID: &parent.ID,
		Accessibility: &models.AccessibilityProfile{
			Entrance: &models.EntranceProps{IsLevel: boolPtr(false)},
		},
	}
	testDB.Create(&child)

	r := httptest.NewRequest(http.MethodGet, "/v1/places/"+child.ID, nil)
	r.SetPathValue("id", child.ID)
	w := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var got models.Place
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Accessibility == nil || got.Accessibility.Entrance == nil {
		t.Fatal("expected entrance in response")
	}
	if got.Accessibility.Entrance.IsInherited {
		t.Error("entrance should not be inherited; child owns it")
	}
	if got.Accessibility.Entrance.IsLevel == nil || *got.Accessibility.Entrance.IsLevel {
		t.Error("expected child's is_level=false to override parent's is_level=true")
	}
}

func TestHandleGetPlace_NoParentReturnsRawData(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	place := models.Place{
		Name:     "Standalone Cafe",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryCafe,
		Rank:     models.RankEstablishment,
		Source:   "test",
		Accessibility: &models.AccessibilityProfile{
			Entrance: &models.EntranceProps{IsLevel: boolPtr(true)},
		},
	}
	testDB.Create(&place)

	r := httptest.NewRequest(http.MethodGet, "/v1/places/"+place.ID, nil)
	r.SetPathValue("id", place.ID)
	w := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var got models.Place
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Accessibility == nil || got.Accessibility.Entrance == nil {
		t.Fatal("expected entrance in accessibility profile")
	}
	if got.Accessibility.Entrance.IsInherited {
		t.Error("entrance should not be inherited for place with no parent")
	}
}

func TestHandleOpenAPISpec(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	w := httptest.NewRecorder()
	handlerForServer(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/yaml") {
		t.Errorf("content-type = %q, want application/yaml", ct)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("body is not valid yaml: %v", err)
	}
	if doc["openapi"] == nil {
		t.Errorf("yaml missing top-level `openapi` key; body: %s", w.Body.String())
	}
}

func TestHandleReadyz_DBReachable(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	newTestServer(t).handleReadyz(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if strings.TrimSpace(w.Body.String()) != `{"status":"ok"}` {
		t.Errorf("body = %q, want {\"status\":\"ok\"}", w.Body.String())
	}
}

func TestHandlePostPlace_StatusDefaultsToActive(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	body, _ := json.Marshal(models.Place{
		Name:     "Test Cafe",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryCafe,
		Rank:     models.RankEstablishment,
		Source:   "test",
	})

	r := httptest.NewRequest(http.MethodPost, "/v1/places", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlerNoAuth(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var place models.Place
	testDB.Last(&place)
	if place.Status != models.PlaceStatusActive {
		t.Errorf("status = %q, want %q", place.Status, models.PlaceStatusActive)
	}
}

func TestHandlePostPlace_ExternalIDsRoundTrip(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	body, _ := json.Marshal(models.Place{
		Name:        "OSM Cafe",
		Lat:         52.5,
		Lng:         13.4,
		Category:    models.CategoryCafe,
		Rank:        models.RankEstablishment,
		Source:      "osm",
		ExternalIDs: models.ExternalIDs{"osm": models.ExternalRef{ID: "node/123456", Confidence: 1.0}},
	})

	r := httptest.NewRequest(http.MethodPost, "/v1/places", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlerNoAuth(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var place models.Place
	testDB.Last(&place)
	if place.ExternalIDs["osm"].ID != "node/123456" {
		t.Errorf("external_ids[osm].id = %q, want %q", place.ExternalIDs["osm"].ID, "node/123456")
	}
	if place.ExternalIDs["osm"].Confidence != 1.0 {
		t.Errorf("external_ids[osm].confidence = %v, want 1.0", place.ExternalIDs["osm"].Confidence)
	}
}

func TestHandlePatchAccessibility_UserVerifiedDefaultsFalse(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	place := models.Place{Name: "Test Cafe", Lat: 52.5, Lng: 13.4, Category: models.CategoryCafe, Rank: models.RankEstablishment, Source: "test"}
	testDB.Create(&place)

	body, _ := json.Marshal(models.AccessibilityProfile{Entrance: &models.EntranceProps{IsLevel: boolPtr(true)}})
	r := httptest.NewRequest(http.MethodPatch, "/v1/places/"+place.ID+"/accessibility", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlerNoAuth(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var profile models.AccessibilityProfile
	testDB.Where("place_id = ?", place.ID).First(&profile)
	if profile.UserVerified {
		t.Error("user_verified should default to false for a new accessibility submission")
	}
}
