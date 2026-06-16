//go:build integration

/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/InWheelOrg/inwheel-api/internal/place"
	"github.com/InWheelOrg/inwheel-api/internal/sources"
	"github.com/InWheelOrg/inwheel-api/internal/testhelpers"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

const fixturePBFPath = "../../testdata/andorra-sample.osm.pbf"

// expectedPOICount is the number of nodes in the Andorra fixture that pass
// osm.Evaluate. Locked in by inspecting the fixture; if the filter rules
// change or the fixture is replaced, this number must be updated.
const expectedPOICount = 976

// pinned is a known POI from the Andorra fixture used to verify that the
// transform → upsert pipeline produces the expected place row.
type pinned struct {
	osmID     int64
	name      string
	category  models.Category
	rank      models.Rank
	lat       float64
	lng       float64
	tagSubset map[string]string
}

var pinnedPOIs = []pinned{
	{
		osmID:    521143390,
		name:     "Supermercat Saint Moritz",
		category: models.CategoryShop,
		rank:     models.RankEstablishment,
		lat:      42.571637,
		lng:      1.484545,
		tagSubset: map[string]string{
			"shop":             "supermarket",
			"name":             "Supermercat Saint Moritz",
			"addr:city":        "Arinsal",
			"addr:street":      "Carretera d'Arinsal",
			"addr:housenumber": "16",
		},
	},
	{
		osmID:    690708548,
		name:     "Farmàcia del Pas",
		category: models.CategoryHealthcare,
		rank:     models.RankEstablishment,
		lat:      42.542391,
		lng:      1.733906,
		tagSubset: map[string]string{
			"amenity":    "pharmacy",
			"healthcare": "pharmacy",
			"name":       "Farmàcia del Pas",
		},
	},
	{
		osmID:    323129883,
		name:     "Telecabina La Massana",
		category: models.CategoryTransport,
		rank:     models.RankLandmark, // public_transport=station promotes transport to landmark
		lat:      42.547295,
		lng:      1.513858,
		tagSubset: map[string]string{
			"public_transport": "station",
			"aerialway":        "station",
			"name":             "Telecabina La Massana",
		},
	},
}

func TestFullImport_AndorraFixture(t *testing.T) {
	ctx := context.Background()
	db, connInfo, cleanup, err := testhelpers.StartPostgresWithConnInfo(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	cfg := config{
		DBHost:     connInfo.Host,
		DBPort:     connInfo.Port,
		DBUser:     connInfo.User,
		DBPassword: connInfo.Password,
		DBName:     connInfo.Name,
		DBSSLMode:  connInfo.SSLMode,
		OSMPBFPath: fixturePBFPath,
	}

	if err := run(ctx, "osm", "full-import", cfg); err != nil {
		t.Fatalf("first import: %v", err)
	}

	t.Run("imports the expected POI count", func(t *testing.T) {
		var count int64
		if err := db.Model(&models.Place{}).Count(&count).Error; err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != expectedPOICount {
			t.Errorf("imported %d places, want %d", count, expectedPOICount)
		}
	})

	for _, p := range pinnedPOIs {
		t.Run("pinned/"+p.name, func(t *testing.T) {
			var got models.Place
			err := db.Where("osm_id = ? AND osm_type = ?", p.osmID, models.OSMNode).First(&got).Error
			if err != nil {
				t.Fatalf("lookup osm_id=%d: %v", p.osmID, err)
			}

			if got.Name != p.name {
				t.Errorf("name = %q, want %q", got.Name, p.name)
			}
			if got.Category != p.category {
				t.Errorf("category = %q, want %q", got.Category, p.category)
			}
			if got.Rank != p.rank {
				t.Errorf("rank = %d, want %d", got.Rank, p.rank)
			}
			if math.Abs(got.Lat-p.lat) > 1e-5 {
				t.Errorf("lat = %f, want ~%f", got.Lat, p.lat)
			}
			if math.Abs(got.Lng-p.lng) > 1e-5 {
				t.Errorf("lng = %f, want ~%f", got.Lng, p.lng)
			}
			if got.Source != "osm" {
				t.Errorf("source = %q, want %q", got.Source, "osm")
			}
			if got.Status != models.PlaceStatusActive {
				t.Errorf("status = %q, want %q", got.Status, models.PlaceStatusActive)
			}
			wantExternalID := fmt.Sprintf("node/%d", p.osmID)
			if got.ExternalIDs["osm"].ID != wantExternalID {
				t.Errorf("external_ids[osm].id = %q, want %q", got.ExternalIDs["osm"].ID, wantExternalID)
			}
			if got.ExternalIDs["osm"].Confidence != 1.0 {
				t.Errorf("external_ids[osm].confidence = %v, want 1.0", got.ExternalIDs["osm"].Confidence)
			}
			for k, v := range p.tagSubset {
				if got.Tags[k] != v {
					t.Errorf("tags[%q] = %q, want %q", k, got.Tags[k], v)
				}
			}
		})
	}

	t.Run("re-import is idempotent on row count", func(t *testing.T) {
		var before int64
		if err := db.Model(&models.Place{}).Count(&before).Error; err != nil {
			t.Fatalf("count before: %v", err)
		}
		if err := run(ctx, "osm", "full-import", cfg); err != nil {
			t.Fatalf("second import: %v", err)
		}
		var after int64
		if err := db.Model(&models.Place{}).Count(&after).Error; err != nil {
			t.Fatalf("count after: %v", err)
		}
		if after != before {
			t.Errorf("row count changed across re-import: before=%d after=%d", before, after)
		}
	})
}

func TestRunCanonical_WritesAccessibilityProfiles(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	hasStep := true
	hasRamp := false
	src := &fakeCanonicalSource{
		emit: []fakeEmit{
			{
				place: models.Place{
					OSMID: 1001, OSMType: models.OSMNode, Name: "Plain",
					Lat: 46.4620, Lng: 6.8400, Category: models.CategoryCafe,
					Rank:        models.RankEstablishment,
					Status:      models.PlaceStatusActive,
					ExternalIDs: models.ExternalIDs{"osm": {ID: "node/1001", Confidence: 1.0}},
				},
			},
			{
				place: models.Place{
					OSMID: 1002, OSMType: models.OSMNode, Name: "Accessible",
					Lat: 46.4621, Lng: 6.8401, Category: models.CategoryCafe,
					Rank:        models.RankEstablishment,
					Status:      models.PlaceStatusActive,
					ExternalIDs: models.ExternalIDs{"osm": {ID: "node/1002", Confidence: 1.0}},
				},
				profile: &models.AccessibilityProfile{OverallStatus: models.StatusAccessible},
			},
			{
				place: models.Place{
					OSMID: 1003, OSMType: models.OSMNode, Name: "Hard Conflict",
					Lat: 46.4622, Lng: 6.8402, Category: models.CategoryCafe,
					Rank:        models.RankEstablishment,
					Status:      models.PlaceStatusActive,
					ExternalIDs: models.ExternalIDs{"osm": {ID: "node/1003", Confidence: 1.0}},
				},
				profile: &models.AccessibilityProfile{
					OverallStatus: models.StatusAccessible,
					Components: models.A11yComponents{{
						Type:          models.ComponentEntrance,
						OverallStatus: models.StatusAccessible,
						Entrance:      &models.EntranceProperties{HasStep: &hasStep, HasRamp: &hasRamp},
					}},
				},
			},
		},
	}

	if err := runCanonical(ctx, src, "full-import", db); err != nil {
		t.Fatalf("runCanonical: %v", err)
	}

	var profiles []models.AccessibilityProfile
	if err := db.Find(&profiles).Error; err != nil {
		t.Fatalf("read profiles: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles (Plain has none), got %d", len(profiles))
	}

	var conflictProfile models.AccessibilityProfile
	if err := db.Joins("JOIN places ON places.id = accessibility_profiles.place_id").
		Where("places.osm_id = ?", 1003).
		First(&conflictProfile).Error; err != nil {
		t.Fatalf("read conflict profile: %v", err)
	}
	if len(conflictProfile.Components) != 1 {
		t.Fatalf("conflict profile components = %d, want 1", len(conflictProfile.Components))
	}
	if conflictProfile.Components[0].OverallStatus != models.StatusLimited {
		t.Errorf("component status = %q, want limited (downgraded from accessible)", conflictProfile.Components[0].OverallStatus)
	}
}

func TestRunCanonical_DoesNotOverwriteUserVerified(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	repo := place.NewRepository(db)
	seed := models.Place{
		OSMID: 2001, OSMType: models.OSMNode, Name: "Verified",
		Lat: 46.5000, Lng: 6.9000, Category: models.CategoryCafe,
		Rank:        models.RankEstablishment,
		Status:      models.PlaceStatusActive,
		ExternalIDs: models.ExternalIDs{"osm": {ID: "node/2001", Confidence: 1.0}},
	}
	if err := db.Create(&seed).Error; err != nil {
		t.Fatalf("seed place: %v", err)
	}
	_, err = repo.UpsertProfile(ctx, seed.ID, &models.AccessibilityProfile{
		OverallStatus: models.StatusInaccessible,
		UserVerified:  true,
	})
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	src := &fakeCanonicalSource{
		emit: []fakeEmit{{
			place: models.Place{
				OSMID: 2001, OSMType: models.OSMNode, Name: "Verified",
				Lat: 46.5000, Lng: 6.9000, Category: models.CategoryCafe,
				Rank:        models.RankEstablishment,
				Status:      models.PlaceStatusActive,
				ExternalIDs: models.ExternalIDs{"osm": {ID: "node/2001", Confidence: 1.0}},
			},
			profile: &models.AccessibilityProfile{OverallStatus: models.StatusAccessible},
		}},
	}
	if err := runCanonical(ctx, src, "full-import", db); err != nil {
		t.Fatalf("runCanonical: %v", err)
	}

	var stored models.AccessibilityProfile
	if err := db.Where("place_id = ?", seed.ID).First(&stored).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if stored.OverallStatus != models.StatusInaccessible {
		t.Errorf("user-verified profile got overwritten: status = %q, want inaccessible", stored.OverallStatus)
	}
	if !stored.UserVerified {
		t.Errorf("user_verified flag was cleared")
	}
}

type fakeEmit struct {
	place   models.Place
	profile *models.AccessibilityProfile
}

type fakeCanonicalSource struct {
	emit []fakeEmit
}

func (f *fakeCanonicalSource) Name() string             { return "fake" }
func (f *fakeCanonicalSource) Kind() sources.SourceKind { return sources.SourceKindCanonical }
func (f *fakeCanonicalSource) FullImport(ctx context.Context, sink sources.Sink) error {
	for _, e := range f.emit {
		if err := sink(ctx, e.place, e.profile); err != nil {
			return err
		}
	}
	return nil
}
