//go:build integration

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

	"github.com/InWheelOrg/inwheel-api/internal/identity"
	"github.com/InWheelOrg/inwheel-api/internal/place"
	"github.com/InWheelOrg/inwheel-api/internal/sources/osm"
	"github.com/InWheelOrg/inwheel-api/internal/unmatched"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

const crossBinaryPBFPath = "../../testdata/andorra-sample.osm.pbf"

func runOSMIngestForCrossBinary(t *testing.T, pbfPath string) {
	t.Helper()
	ctx := t.Context()

	src := &osm.Source{PBFPath: pbfPath}
	placesRepo := place.NewRepository(testDB)
	unmatchedRepo := unmatched.NewRepository(testDB)

	const batchSize = 1000
	var buf []models.Place
	flush := func() {
		if len(buf) == 0 {
			return
		}
		if err := placesRepo.UpsertBatch(ctx, buf); err != nil {
			t.Fatalf("upsert batch: %v", err)
		}
		buf = buf[:0]
	}
	sink := func(_ context.Context, p models.Place) error {
		buf = append(buf, p)
		if len(buf) >= batchSize {
			flush()
		}
		return nil
	}

	if err := src.FullImport(ctx, sink); err != nil {
		t.Fatalf("OSM full import: %v", err)
	}
	flush()

	var touched []string
	if err := testDB.Model(&models.Place{}).Pluck("id", &touched).Error; err != nil {
		t.Fatalf("collect touched IDs: %v", err)
	}
	sweeper := &identity.Sweeper{
		Candidates: placesRepo,
		Places:     placesRepo,
		Queue:      unmatchedRepo,
		Now:        time.Now,
	}
	if _, err := sweeper.Sweep(ctx, touched); err != nil {
		t.Logf("sweep: %v", err)
	}
}

func TestCrossBinary_OSMIngest_PlaceReadableViaAPI(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	runOSMIngestForCrossBinary(t, crossBinaryPBFPath)

	ts := httptest.NewServer(handlerForServer(t, newTestServer(t)))
	t.Cleanup(ts.Close)

	var pinned models.Place
	if err := testDB.
		Where("osm_id = ? AND osm_type = ?", int64(521143390), models.OSMNode).
		First(&pinned).Error; err != nil {
		t.Fatalf("locate pinned place: %v", err)
	}

	resp, err := http.Get(ts.URL + "/v1/places/" + pinned.ID)
	if err != nil {
		t.Fatalf("GET /v1/places/%s: %v", pinned.ID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var got models.Place
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got.ID != pinned.ID {
		t.Errorf("id = %q, want %q", got.ID, pinned.ID)
	}
	if got.Name != "Supermercat Saint Moritz" {
		t.Errorf("name = %q, want %q", got.Name, "Supermercat Saint Moritz")
	}
	if got.Category != models.CategoryShop {
		t.Errorf("category = %q, want %q", got.Category, models.CategoryShop)
	}
	if got.Source != "osm" {
		t.Errorf("source = %q, want %q", got.Source, "osm")
	}
	if got.Status != models.PlaceStatusActive {
		t.Errorf("status = %q, want %q", got.Status, models.PlaceStatusActive)
	}

	osmRef, ok := got.ExternalIDs["osm"]
	if !ok {
		t.Fatalf("response missing external_ids.osm: %#v", got.ExternalIDs)
	}
	if osmRef.ID != "node/521143390" {
		t.Errorf("external_ids.osm.id = %q, want %q", osmRef.ID, "node/521143390")
	}
	if osmRef.Confidence != 1.0 {
		t.Errorf("external_ids.osm.confidence = %v, want 1.0", osmRef.Confidence)
	}

	if got.Lat == 0 && got.Lng == 0 {
		t.Errorf("lat/lng both zero — geometry may not have scanned correctly")
	}

	if got.Tags["addr:street"] != "Carretera d'Arinsal" {
		t.Errorf("tags[addr:street] = %q, want %q", got.Tags["addr:street"], "Carretera d'Arinsal")
	}
	if got.Tags["addr:housenumber"] != "16" {
		t.Errorf("tags[addr:housenumber] = %q, want %q", got.Tags["addr:housenumber"], "16")
	}
}
