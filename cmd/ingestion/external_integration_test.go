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

	"github.com/InWheelOrg/inwheel-api/internal/identity"
	"github.com/InWheelOrg/inwheel-api/internal/sources"
	"github.com/InWheelOrg/inwheel-api/internal/testhelpers"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// syntheticSource is an in-test ExternalFullImporter. The dispatch picks it up
// via type assertion and routes each emitted Record through identity.Resolver.
type syntheticSource struct {
	records []identity.Record
}

func (s *syntheticSource) Name() string             { return "synthetic" }
func (s *syntheticSource) Kind() sources.SourceKind { return sources.SourceKindExternal }
func (s *syntheticSource) FullImport(ctx context.Context, sink sources.RecordSink) error {
	for _, r := range s.records {
		if err := sink(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func TestExternalIngest_RoutesAllThreeOutcomes(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	seedPlaces := []models.Place{
		{
			Name:     "Pascal",
			Lat:      46.4628,
			Lng:      6.8417,
			Category: models.CategoryCafe,
			Rank:     models.RankEstablishment,
			Source:   "osm",
			Status:   models.PlaceStatusActive,
			Tags: models.PlaceTags{
				"addr:street":      "Rue du Simplon",
				"addr:housenumber": "10",
			},
			ExternalIDs: models.ExternalIDs{
				"osm": {ID: "node/100", Confidence: 1.0},
			},
		},
		{
			Name:     "Lavaux",
			Lat:      46.462575,
			Lng:      6.8417,
			Category: models.CategoryCafe,
			Rank:     models.RankEstablishment,
			Source:   "osm",
			Status:   models.PlaceStatusActive,
			ExternalIDs: models.ExternalIDs{
				"osm": {ID: "node/101", Confidence: 1.0},
			},
		},
	}
	if err := db.Create(&seedPlaces).Error; err != nil {
		t.Fatalf("seed places: %v", err)
	}

	records := []identity.Record{
		{
			Source: "synthetic", SourceID: "conf-1",
			Name: "Pascal", Lat: 46.4628, Lng: 6.8417, Category: models.CategoryCafe,
			Street: "Rue du Simplon", HouseNumber: "10",
			Payload: json.RawMessage(`{"why":"confident"}`),
		},
		{
			Source: "synthetic", SourceID: "low-1",
			Name: "Lavaux", Lat: 46.4628, Lng: 6.8417, Category: models.CategoryCafe,
			Payload: json.RawMessage(`{"why":"low"}`),
		},
		{
			Source: "synthetic", SourceID: "miss-1",
			Name: "Ghost", Lat: 47.5000, Lng: 7.5000, Category: models.CategoryCafe,
			Street: "Quai du Léman", HouseNumber: "42",
			Payload: json.RawMessage(`{"why":"miss"}`),
		},
	}

	src := &syntheticSource{records: records}
	if err := runPipeline(ctx, src, "full-import", db); err != nil {
		t.Fatalf("runPipeline: %v", err)
	}

	t.Run("confident attaches external ref to matched place", func(t *testing.T) {
		var got models.Place
		if err := db.Where("name = ?", "Pascal").First(&got).Error; err != nil {
			t.Fatalf("lookup Pascal: %v", err)
		}
		ref, ok := got.ExternalIDs["synthetic"]
		if !ok {
			t.Fatalf("Pascal.external_ids has no 'synthetic' key: %#v", got.ExternalIDs)
		}
		if ref.ID != "conf-1" {
			t.Errorf("ref.ID = %q, want %q", ref.ID, "conf-1")
		}
		if ref.Confidence < 0.80 {
			t.Errorf("ref.Confidence = %v, want >= 0.80 (confident)", ref.Confidence)
		}
	})

	t.Run("low-confidence attaches external ref with low confidence", func(t *testing.T) {
		var got models.Place
		if err := db.Where("name = ?", "Lavaux").First(&got).Error; err != nil {
			t.Fatalf("lookup Lavaux: %v", err)
		}
		ref, ok := got.ExternalIDs["synthetic"]
		if !ok {
			t.Fatalf("Lavaux.external_ids has no 'synthetic' key: %#v", got.ExternalIDs)
		}
		if ref.ID != "low-1" {
			t.Errorf("ref.ID = %q, want %q", ref.ID, "low-1")
		}
		if ref.Confidence < 0.55 || ref.Confidence >= 0.80 {
			t.Errorf("ref.Confidence = %v, want in [0.55, 0.80)", ref.Confidence)
		}
	})

	t.Run("no-match enqueues in unmatched_external with full matchable signal", func(t *testing.T) {
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatalf("get sql.DB: %v", err)
		}
		var (
			gotName, gotCategory, gotStreet, gotHouse string
			gotLat, gotLng                            float64
		)
		row := sqlDB.QueryRowContext(ctx,
			`SELECT name, category, street, housenumber, lat, lng
			 FROM unmatched_external WHERE source = $1 AND source_id = $2`,
			"synthetic", "miss-1",
		)
		if err := row.Scan(&gotName, &gotCategory, &gotStreet, &gotHouse, &gotLat, &gotLng); err != nil {
			t.Fatalf("scan unmatched row: %v", err)
		}
		if gotName != "Ghost" {
			t.Errorf("name = %q, want %q", gotName, "Ghost")
		}
		if gotCategory != string(models.CategoryCafe) {
			t.Errorf("category = %q, want %q", gotCategory, models.CategoryCafe)
		}
		if gotStreet != "Quai du Léman" {
			t.Errorf("street = %q, want %q", gotStreet, "Quai du Léman")
		}
		if gotHouse != "42" {
			t.Errorf("housenumber = %q, want %q", gotHouse, "42")
		}
		if gotLat != 47.5000 || gotLng != 7.5000 {
			t.Errorf("lat/lng = %v/%v, want 47.5/7.5", gotLat, gotLng)
		}
	})
}
