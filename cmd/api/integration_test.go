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
	"testing"

	"github.com/InWheelOrg/inwheel-server/internal/testhelpers"
	"github.com/InWheelOrg/inwheel-server/pkg/models"
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
	testDB.Exec("TRUNCATE places, accessibility_profiles CASCADE")
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

	r := httptest.NewRequest(http.MethodPost, "/places", bytes.NewReader(body))
	w := httptest.NewRecorder()
	(&Server{db: testDB}).handlePostPlace(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var place models.Place
	testDB.Preload("Accessibility").Last(&place)

	if place.Accessibility == nil {
		t.Fatal("expected accessibility profile to be created")
	}
	if !place.Accessibility.NeedsAudit {
		t.Error("expected NeedsAudit=true when accessibility is included in POST")
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

	r := httptest.NewRequest(http.MethodPost, "/places", bytes.NewReader(body))
	w := httptest.NewRecorder()
	(&Server{db: testDB}).handlePostPlace(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var place models.Place
	testDB.Preload("Accessibility").Last(&place)

	if place.Accessibility != nil {
		t.Error("expected no accessibility profile when none submitted")
	}
}

func TestHandlePatchAccessibility_CreatePath(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	place := models.Place{Name: "Test Place", Lat: 52.5, Lng: 13.4, Category: models.CategoryCafe, Source: "test"}
	testDB.Create(&place)

	body, _ := json.Marshal(models.AccessibilityProfile{
		OverallStatus: models.StatusLimited,
	})

	r := httptest.NewRequest(http.MethodPatch, "/places/"+place.ID+"/accessibility", bytes.NewReader(body))
	r.SetPathValue("id", place.ID)
	w := httptest.NewRecorder()
	(&Server{db: testDB}).handlePatchAccessibility(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var profile models.AccessibilityProfile
	testDB.Where("place_id = ?", place.ID).First(&profile)

	if !profile.NeedsAudit {
		t.Error("expected NeedsAudit=true on first PATCH")
	}
	if profile.DataVersion != 1 {
		t.Errorf("DataVersion = %d, want 1 on first PATCH", profile.DataVersion)
	}
}

func TestHandlePatchAccessibility_UpdateIncrementsVersion(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	place := models.Place{
		Name:     "Test Place",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryCafe,
		Source:   "test",
		Accessibility: &models.AccessibilityProfile{
			OverallStatus: models.StatusAccessible,
			NeedsAudit:    false,
			DataVersion:   1,
		},
	}
	testDB.Create(&place)

	body, _ := json.Marshal(models.AccessibilityProfile{
		OverallStatus: models.StatusLimited,
	})

	r := httptest.NewRequest(http.MethodPatch, "/places/"+place.ID+"/accessibility", bytes.NewReader(body))
	r.SetPathValue("id", place.ID)
	w := httptest.NewRecorder()
	(&Server{db: testDB}).handlePatchAccessibility(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var profile models.AccessibilityProfile
	testDB.Where("place_id = ?", place.ID).First(&profile)

	if !profile.NeedsAudit {
		t.Error("expected NeedsAudit=true after update")
	}
	if profile.DataVersion != 2 {
		t.Errorf("DataVersion = %d, want 2 after update", profile.DataVersion)
	}
	if profile.OverallStatus != models.StatusLimited {
		t.Errorf("OverallStatus = %s, want limited", profile.OverallStatus)
	}
}
