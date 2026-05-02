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

	"github.com/InWheelOrg/inwheel-server/internal/a11y"
	"github.com/InWheelOrg/inwheel-server/internal/middleware"
	"github.com/InWheelOrg/inwheel-server/internal/testhelpers"
	"github.com/InWheelOrg/inwheel-server/pkg/models"
	"golang.org/x/time/rate"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

var testDB *gorm.DB

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

// run is extracted from TestMain so defer-based cleanup executes before os.Exit.
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

// truncate clears all test data between tests to prevent state bleed.
func truncate(t *testing.T) {
	t.Helper()
	testDB.Exec("TRUNCATE places, accessibility_profiles, api_keys, write_logs CASCADE")
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	ctx := t.Context()
	return &Server{
		db:         testDB,
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
		Source:   "test",
		Accessibility: &models.AccessibilityProfile{
			OverallStatus: models.StatusAccessible,
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

func TestHandlePostPlace_HardConflictReturns422(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	// step with no ramp + accessible = hard self-contradiction
	stepHeight := 0.1
	hasStep := true
	hasRamp := false
	body, _ := json.Marshal(models.Place{
		Name:     "Conflicting Cafe",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryCafe,
		Source:   "test",
		Accessibility: &models.AccessibilityProfile{
			OverallStatus: models.StatusAccessible,
			Components: models.A11yComponents{
				{
					Type:          models.ComponentEntrance,
					OverallStatus: models.StatusAccessible,
					Entrance: &models.EntranceProperties{
						HasStep:    &hasStep,
						StepHeight: &stepHeight,
						HasRamp:    &hasRamp,
					},
				},
			},
		},
	})

	r := httptest.NewRequest(http.MethodPost, "/v1/places", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlerNoAuth(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Error     string `json:"error"`
		Conflicts []struct {
			Component string `json:"component"`
			Reason    string `json:"reason"`
		} `json:"conflicts"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode 422 response: %v", err)
	}
	if len(resp.Conflicts) == 0 {
		t.Error("expected conflicts in 422 response body")
	}
}

func TestHandlePostPlace_InformationalFlagsAllowed(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	// narrow width is informational only — should not block the write
	narrowWidth := 0.75
	body, _ := json.Marshal(models.Place{
		Name:     "Narrow Cafe",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryCafe,
		Source:   "test",
		Accessibility: &models.AccessibilityProfile{
			OverallStatus: models.StatusAccessible,
			Components: models.A11yComponents{
				{
					Type:          models.ComponentEntrance,
					OverallStatus: models.StatusAccessible,
					Entrance:      &models.EntranceProperties{Width: &narrowWidth},
				},
			},
		},
	})

	r := httptest.NewRequest(http.MethodPost, "/v1/places", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlerNoAuth(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	// Verify the flag was stored
	var place models.Place
	testDB.Preload("Accessibility").Last(&place)
	if place.Accessibility == nil || len(place.Accessibility.Components) == 0 {
		t.Fatal("expected accessibility with components")
	}
	flags := place.Accessibility.Components[0].AuditFlags
	found := false
	for _, f := range flags {
		if f == "narrow width (0.8m required)" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected narrow width flag to be stored, got flags: %v", flags)
	}
}

func TestHandlePatchAccessibility_PlaceNotFound(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	const nonExistentID = "00000000-0000-0000-0000-000000000000"
	body, _ := json.Marshal(models.AccessibilityProfile{OverallStatus: models.StatusAccessible})

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

	place := models.Place{Name: "Test Place", Lat: 52.5, Lng: 13.4, Category: models.CategoryCafe, Source: "test"}
	testDB.Create(&place)

	body, _ := json.Marshal(models.AccessibilityProfile{
		OverallStatus: models.StatusLimited,
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
	if profile.OverallStatus != models.StatusLimited {
		t.Errorf("OverallStatus = %s, want limited", profile.OverallStatus)
	}
}

func TestHandlePatchAccessibility_UpdatesExistingProfile(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	place := models.Place{
		Name:     "Test Place",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryCafe,
		Source:   "test",
		Accessibility: &models.AccessibilityProfile{
			OverallStatus: models.StatusAccessible,
		},
	}
	testDB.Create(&place)

	body, _ := json.Marshal(models.AccessibilityProfile{
		OverallStatus: models.StatusLimited,
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

	if profile.OverallStatus != models.StatusLimited {
		t.Errorf("OverallStatus = %s, want limited", profile.OverallStatus)
	}
}

func TestHandlePatchAccessibility_ConflictReturns422(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	place := models.Place{Name: "Test Place", Lat: 52.5, Lng: 13.4, Category: models.CategoryCafe, Source: "test"}
	testDB.Create(&place)

	stepHeight := 0.1
	hasStep := true
	hasRamp := false
	body, _ := json.Marshal(models.AccessibilityProfile{
		OverallStatus: models.StatusAccessible,
		Components: models.A11yComponents{
			{
				Type:          models.ComponentEntrance,
				OverallStatus: models.StatusAccessible,
				Entrance: &models.EntranceProperties{
					HasStep:    &hasStep,
					StepHeight: &stepHeight,
					HasRamp:    &hasRamp,
				},
			},
		},
	})

	r := httptest.NewRequest(http.MethodPatch, "/v1/places/"+place.ID+"/accessibility", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.SetPathValue("id", place.ID)
	w := httptest.NewRecorder()
	handlerNoAuth(t, newTestServer(t)).ServeHTTP(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Error     string `json:"error"`
		Conflicts []any  `json:"conflicts"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode 422 response: %v", err)
	}
	if len(resp.Conflicts) == 0 {
		t.Error("expected conflicts in 422 response body")
	}
}

func TestHandleGetPlace_ReturnsPlaceWithAccessibility(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	place := models.Place{
		Name:     "Test Cafe",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryCafe,
		Source:   "test",
		Accessibility: &models.AccessibilityProfile{
			OverallStatus: models.StatusAccessible,
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

	hasSpaces := true
	parent := models.Place{
		Name:     "Test Mall",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryMall,
		Source:   "test",
		Accessibility: &models.AccessibilityProfile{
			OverallStatus: models.StatusAccessible,
			Components: models.A11yComponents{
				{Type: models.ComponentParking, OverallStatus: models.StatusAccessible, Parking: &models.ParkingProperties{HasDisabledSpaces: &hasSpaces}},
			},
		},
	}
	testDB.Create(&parent)

	child := models.Place{
		Name:     "Test Shop",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryShop,
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

	var inherited *models.A11yComponent
	for i := range got.Accessibility.Components {
		if got.Accessibility.Components[i].Type == models.ComponentParking {
			inherited = &got.Accessibility.Components[i]
			break
		}
	}
	if inherited == nil {
		t.Fatal("expected inherited parking component in effective profile")
	}
	if !inherited.IsInherited {
		t.Error("parking component should be marked is_inherited=true")
	}
	if inherited.SourceID != parent.ID {
		t.Errorf("source_id = %q, want %q", inherited.SourceID, parent.ID)
	}
}

func TestHandleGetPlace_ChildOverridesParentComponent(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	parent := models.Place{
		Name:     "Test Mall",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryMall,
		Source:   "test",
		Accessibility: &models.AccessibilityProfile{
			OverallStatus: models.StatusAccessible,
			Components: models.A11yComponents{
				{Type: models.ComponentEntrance, OverallStatus: models.StatusAccessible},
			},
		},
	}
	testDB.Create(&parent)

	child := models.Place{
		Name:     "Test Shop",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryShop,
		Source:   "test",
		ParentID: &parent.ID,
		Accessibility: &models.AccessibilityProfile{
			OverallStatus: models.StatusInaccessible,
			Components: models.A11yComponents{
				{Type: models.ComponentEntrance, OverallStatus: models.StatusInaccessible},
			},
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
	if got.Accessibility == nil {
		t.Fatal("expected accessibility profile in response")
	}

	var entranceCount int
	for _, c := range got.Accessibility.Components {
		if c.Type == models.ComponentEntrance {
			entranceCount++
			if c.IsInherited {
				t.Error("entrance should not be inherited — child owns it")
			}
			if c.OverallStatus != models.StatusInaccessible {
				t.Errorf("entrance status = %q, want inaccessible", c.OverallStatus)
			}
		}
	}
	if entranceCount != 1 {
		t.Errorf("expected exactly 1 entrance component, got %d", entranceCount)
	}
}

func TestHandleGetPlace_NoParentReturnsRawData(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	place := models.Place{
		Name:     "Standalone Cafe",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryCafe,
		Source:   "test",
		Accessibility: &models.AccessibilityProfile{
			OverallStatus: models.StatusAccessible,
			Components: models.A11yComponents{
				{Type: models.ComponentEntrance, OverallStatus: models.StatusAccessible},
			},
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
		t.Fatal("expected accessibility profile")
	}
	if len(got.Accessibility.Components) != 1 {
		t.Errorf("expected 1 component, got %d", len(got.Accessibility.Components))
	}
	if got.Accessibility.Components[0].IsInherited {
		t.Error("component should not be inherited for place with no parent")
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
