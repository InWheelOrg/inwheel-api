//go:build integration

/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/InWheelOrg/inwheel-api/internal/identity"
	"github.com/InWheelOrg/inwheel-api/internal/place"
	"github.com/InWheelOrg/inwheel-api/internal/testhelpers"
	"github.com/InWheelOrg/inwheel-api/internal/unmatched"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// TestResolverEnqueueIsConsumableBySweep proves the Resolver writes queue rows
// that the Sweeper can actually reconstruct into a Record and match. If
// Resolver ever drops a matchable column on enqueue, this test fails because
// the Sweeper rebuilds an empty Record and scores 0 against the seeded place.
func TestResolverEnqueueIsConsumableBySweep(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	placesRepo := place.NewRepository(db)
	queueRepo := unmatched.NewRepository(db)
	clock := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

	resolver := &identity.Resolver{
		Candidates: placesRepo,
		Places:     placesRepo,
		Unmatched:  queueRepo,
		Now:        func() time.Time { return clock },
	}

	rec := identity.Record{
		Source: "synthetic", SourceID: "round-trip-1",
		Name:        "Pascal",
		Lat:         46.4628,
		Lng:         6.8417,
		Category:    models.CategoryCafe,
		Street:      "Rue du Simplon",
		HouseNumber: "10",
		Payload:     json.RawMessage(`{"why":"no-place-yet"}`),
	}
	d, err := resolver.Resolve(ctx, rec)
	if err != nil {
		t.Fatalf("Resolve (pre-place): %v", err)
	}
	if d.Kind != identity.KindNoMatch {
		t.Fatalf("Resolve.Kind = %v, want KindNoMatch (no places exist yet)", d.Kind)
	}

	seed := models.Place{
		Name:     "Pascal",
		Lat:      46.4628,
		Lng:      6.8417,
		Category: models.CategoryCafe,
		Source:   "osm",
		Status:   models.PlaceStatusActive,
		Tags: models.PlaceTags{
			"addr:street":      "Rue du Simplon",
			"addr:housenumber": "10",
		},
		ExternalIDs: models.ExternalIDs{
			"osm": {ID: "node/200", Confidence: 1.0},
		},
	}
	if err := db.Create(&seed).Error; err != nil {
		t.Fatalf("seed place: %v", err)
	}

	sweeper := &identity.Sweeper{
		Candidates: placesRepo,
		Places:     placesRepo,
		Queue:      queueRepo,
		Now:        func() time.Time { return clock.Add(time.Hour) },
	}
	res, err := sweeper.Sweep(ctx, []string{seed.ID})
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Confident != 1 {
		t.Fatalf("Sweep result = %+v, want Confident=1 (proves the queue row carried full signal)", res)
	}

	var got models.Place
	if err := db.First(&got, "id = ?", seed.ID).Error; err != nil {
		t.Fatalf("reload seeded place: %v", err)
	}
	ref, ok := got.ExternalIDs["synthetic"]
	if !ok {
		t.Fatalf("seeded place external_ids has no 'synthetic' key: %#v", got.ExternalIDs)
	}
	if ref.ID != "round-trip-1" {
		t.Errorf("ref.ID = %q, want %q", ref.ID, "round-trip-1")
	}
	if ref.Confidence < 0.80 {
		t.Errorf("ref.Confidence = %v, want >= 0.80", ref.Confidence)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	var count int
	if err := sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM unmatched_external WHERE source = $1 AND source_id = $2",
		"synthetic", "round-trip-1",
	).Scan(&count); err != nil {
		t.Fatalf("count remaining queue rows: %v", err)
	}
	if count != 0 {
		t.Errorf("queue row not deleted by sweep (count=%d)", count)
	}
}
