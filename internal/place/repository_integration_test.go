//go:build integration

/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package place_test

import (
	"context"
	"testing"

	"github.com/InWheelOrg/inwheel-api/internal/place"
	"github.com/InWheelOrg/inwheel-api/internal/testhelpers"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

func TestUpsertBatch_InsertsNewPlaces(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	repo := place.NewRepository(db)

	places := []models.Place{
		{OSMID: 1, OSMType: models.OSMNode, Name: "A", Lat: 60, Lng: 24, Category: models.CategoryRestaurant, Rank: models.RankEstablishment, Source: "osm", Status: models.PlaceStatusActive, ExternalIDs: models.ExternalIDs{"osm": models.ExternalRef{ID: "node/1", Confidence: 1.0}}},
		{OSMID: 2, OSMType: models.OSMNode, Name: "B", Lat: 61, Lng: 25, Category: models.CategoryCafe, Rank: models.RankEstablishment, Source: "osm", Status: models.PlaceStatusActive, ExternalIDs: models.ExternalIDs{"osm": models.ExternalRef{ID: "node/2", Confidence: 1.0}}},
	}

	if err := repo.UpsertBatch(ctx, places); err != nil {
		t.Fatalf("upsert batch: %v", err)
	}

	var count int64
	db.Model(&models.Place{}).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 rows, got %d", count)
	}
	var inserted models.Place
	if err := db.Where("osm_id = ?", 1).First(&inserted).Error; err != nil {
		t.Fatalf("fetch inserted place: %v", err)
	}
	if inserted.ExternalIDs["osm"].ID != "node/1" {
		t.Errorf("external_ids[osm].id = %q, want %q", inserted.ExternalIDs["osm"].ID, "node/1")
	}
	if inserted.ExternalIDs["osm"].Confidence != 1.0 {
		t.Errorf("external_ids[osm].confidence = %v, want 1.0", inserted.ExternalIDs["osm"].Confidence)
	}
}

func TestUpsertBatch_UpdatesExistingPlace(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	repo := place.NewRepository(db)

	first := []models.Place{{OSMID: 42, OSMType: models.OSMNode, Name: "Original", Lat: 60, Lng: 24, Category: models.CategoryRestaurant, Rank: models.RankEstablishment, Source: "osm", Status: models.PlaceStatusActive, ExternalIDs: models.ExternalIDs{"osm": models.ExternalRef{ID: "node/42", Confidence: 1.0}}}}
	if err := repo.UpsertBatch(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	second := []models.Place{{OSMID: 42, OSMType: models.OSMNode, Name: "Renamed", Lat: 60.5, Lng: 24.5, Category: models.CategoryRestaurant, Rank: models.RankEstablishment, Source: "osm", Status: models.PlaceStatusActive, ExternalIDs: models.ExternalIDs{"osm": models.ExternalRef{ID: "node/42", Confidence: 1.0}}}}
	if err := repo.UpsertBatch(ctx, second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	var got models.Place
	if err := db.Where("osm_id = ?", 42).First(&got).Error; err != nil {
		t.Fatalf("fetch updated row: %v", err)
	}
	if got.Name != "Renamed" {
		t.Errorf("name was not updated: got %q", got.Name)
	}
	if got.Lat != 60.5 || got.Lng != 24.5 {
		t.Errorf("coords were not updated: got (%v, %v)", got.Lat, got.Lng)
	}
	if got.ExternalIDs["osm"].ID != "node/42" {
		t.Errorf("external_ids[osm].id = %q, want %q", got.ExternalIDs["osm"].ID, "node/42")
	}
	if got.ExternalIDs["osm"].Confidence != 1.0 {
		t.Errorf("external_ids[osm].confidence = %v, want 1.0", got.ExternalIDs["osm"].Confidence)
	}
}

func TestUpsertBatch_EmptySliceIsNoOp(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	repo := place.NewRepository(db)
	if err := repo.UpsertBatch(ctx, nil); err != nil {
		t.Fatalf("nil batch should be a no-op, got error: %v", err)
	}
	if err := repo.UpsertBatch(ctx, []models.Place{}); err != nil {
		t.Fatalf("empty batch should be a no-op, got error: %v", err)
	}
}
