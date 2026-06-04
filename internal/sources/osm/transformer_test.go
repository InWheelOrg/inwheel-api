/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package osm

import (
	"testing"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

func TestTransformNode(t *testing.T) {
	tags := map[string]string{
		"amenity": "restaurant",
		"name":    "Ravintola Tor",
	}

	place, err := TransformNode(123, 60.1699, 24.9384, tags, models.CategoryRestaurant)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if place.OSMID != 123 {
		t.Errorf("osm_id: got %d want 123", place.OSMID)
	}
	if place.OSMType != models.OSMNode {
		t.Errorf("osm_type: got %q want %q", place.OSMType, models.OSMNode)
	}
	if place.Name != "Ravintola Tor" {
		t.Errorf("name: got %q", place.Name)
	}
	if place.Lat != 60.1699 || place.Lng != 24.9384 {
		t.Errorf("coords: got (%v, %v)", place.Lat, place.Lng)
	}
	if place.Category != models.CategoryRestaurant {
		t.Errorf("category: got %q", place.Category)
	}
	if place.Rank != models.RankEstablishment {
		t.Errorf("rank: got %d want %d", place.Rank, models.RankEstablishment)
	}
	if place.ExternalIDs["osm"].ID != "node/123" {
		t.Errorf("external_ids[osm].id: got %q want node/123", place.ExternalIDs["osm"].ID)
	}
	if place.ExternalIDs["osm"].Confidence != 1.0 {
		t.Errorf("external_ids[osm].confidence: got %v want 1.0", place.ExternalIDs["osm"].Confidence)
	}
	if !place.ExternalIDs["osm"].MatchedAt.IsZero() {
		t.Errorf("external_ids[osm].matched_at: expected zero for OSM entry, got %v", place.ExternalIDs["osm"].MatchedAt)
	}
	if place.Source != "osm" {
		t.Errorf("source: got %q want osm", place.Source)
	}
	if place.Status != models.PlaceStatusActive {
		t.Errorf("status: got %q want active", place.Status)
	}
}

func TestTransformNode_PreservesTags(t *testing.T) {
	tags := map[string]string{
		"amenity":   "restaurant",
		"name":      "Burger Place",
		"addr:city": "Helsinki",
	}

	place, err := TransformNode(1, 60, 24, tags, models.CategoryRestaurant)
	if err != nil {
		t.Fatal(err)
	}

	for k, v := range tags {
		if got := place.Tags[k]; got != v {
			t.Errorf("tag %q: got %q want %q", k, got, v)
		}
	}
}

func TestTransformNode_EmptyCategoryReturnsError(t *testing.T) {
	_, err := TransformNode(1, 60, 24, map[string]string{"amenity": "restaurant"}, "")
	if err == nil {
		t.Fatal("expected error for empty category, got nil")
	}
}

func TestDeriveRank(t *testing.T) {
	cases := []struct {
		name     string
		category models.Category
		tags     map[string]string
		want     models.Rank
	}{
		{"hospital is landmark", models.CategoryHealthcare, map[string]string{"amenity": "hospital"}, models.RankLandmark},
		{"clinic is establishment", models.CategoryHealthcare, map[string]string{"amenity": "clinic"}, models.RankEstablishment},
		{"university is landmark", models.CategoryEducation, map[string]string{"amenity": "university"}, models.RankLandmark},
		{"school is establishment", models.CategoryEducation, map[string]string{"amenity": "school"}, models.RankEstablishment},
		{"bus_station is landmark", models.CategoryTransport, map[string]string{"amenity": "bus_station"}, models.RankLandmark},
		{"ferry_terminal is landmark", models.CategoryTransport, map[string]string{"amenity": "ferry_terminal"}, models.RankLandmark},
		{"public_transport station is landmark", models.CategoryTransport, map[string]string{"public_transport": "station"}, models.RankLandmark},
		{"taxi is establishment", models.CategoryTransport, map[string]string{"amenity": "taxi"}, models.RankEstablishment},
		{"toilet is feature", models.CategoryToilet, map[string]string{"amenity": "toilets"}, models.RankFeature},
		{"atm is feature", models.CategoryFinance, map[string]string{"amenity": "atm"}, models.RankFeature},
		{"bank is establishment", models.CategoryFinance, map[string]string{"amenity": "bank"}, models.RankEstablishment},
		{"shelter is feature", models.CategorySocial, map[string]string{"amenity": "shelter"}, models.RankFeature},
		{"restaurant is establishment", models.CategoryRestaurant, map[string]string{"amenity": "restaurant"}, models.RankEstablishment},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DeriveRank(c.category, c.tags)
			if got != c.want {
				t.Fatalf("rank: got %d want %d", got, c.want)
			}
		})
	}
}
