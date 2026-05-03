//go:build integration

/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package geo

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/InWheelOrg/inwheel-api/internal/testhelpers"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
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
	testDB.Exec("TRUNCATE places CASCADE")
}

func TestFindNearbyPlaces(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	// Berlin city centre (~52.5200°N, 13.4050°E)
	berlin := models.Place{Name: "Berlin", Lat: 52.52, Lng: 13.405, Category: "cafe", Source: "test"}
	// London (~51.5074°N, 0.1278°W) — ~930 km away
	london := models.Place{Name: "London", Lat: 51.507, Lng: -0.127, Category: "cafe", Source: "test"}

	testDB.Create(&berlin)
	testDB.Create(&london)

	// 500 m radius centred on Berlin — should return only Berlin.
	places, err := FindNearbyPlaces(testDB, 13.405, 52.52, 500)
	if err != nil {
		t.Fatalf("FindNearbyPlaces error: %v", err)
	}

	if len(places) != 1 {
		t.Fatalf("got %d places, want 1", len(places))
	}
	if places[0].Name != "Berlin" {
		t.Errorf("got place %q, want Berlin", places[0].Name)
	}
}

func TestFindPlacesInBoundingBox(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	berlin := models.Place{Name: "Berlin", Lat: 52.52, Lng: 13.405, Category: "cafe", Source: "test"}
	london := models.Place{Name: "London", Lat: 51.507, Lng: -0.127, Category: "cafe", Source: "test"}

	testDB.Create(&berlin)
	testDB.Create(&london)

	// Bounding box covering central Europe but not the UK.
	places, err := FindPlacesInBoundingBox(testDB, 10.0, 50.0, 15.0, 55.0)
	if err != nil {
		t.Fatalf("FindPlacesInBoundingBox error: %v", err)
	}

	if len(places) != 1 {
		t.Fatalf("got %d places, want 1", len(places))
	}
	if places[0].Name != "Berlin" {
		t.Errorf("got place %q, want Berlin", places[0].Name)
	}
}
