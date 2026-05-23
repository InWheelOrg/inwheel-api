/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package osm

import (
	"testing"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

func TestEvaluate(t *testing.T) {
	cases := []struct {
		name     string
		tags     map[string]string
		want     models.Category
		included bool
	}{
		{"restaurant", map[string]string{"amenity": "restaurant"}, models.CategoryRestaurant, true},
		{"cafe", map[string]string{"amenity": "cafe"}, models.CategoryCafe, true},
		{"bar", map[string]string{"amenity": "bar"}, models.CategoryBar, true},
		{"pub maps to bar", map[string]string{"amenity": "pub"}, models.CategoryBar, true},
		{"hospital", map[string]string{"amenity": "hospital"}, models.CategoryHealthcare, true},
		{"school", map[string]string{"amenity": "school"}, models.CategoryEducation, true},
		{"atm", map[string]string{"amenity": "atm"}, models.CategoryFinance, true},
		{"cinema", map[string]string{"amenity": "cinema"}, models.CategoryEntertainment, true},
		{"townhall", map[string]string{"amenity": "townhall"}, models.CategoryGovernment, true},
		{"bus_station", map[string]string{"amenity": "bus_station"}, models.CategoryTransport, true},
		{"social_facility", map[string]string{"amenity": "social_facility"}, models.CategorySocial, true},
		{"toilets", map[string]string{"amenity": "toilets"}, models.CategoryToilet, true},
		{"place_of_worship", map[string]string{"amenity": "place_of_worship"}, models.CategoryWorship, true},
		{"public_transport station", map[string]string{"public_transport": "station"}, models.CategoryTransport, true},
		{"shop generic", map[string]string{"shop": "supermarket"}, models.CategoryShop, true},
		{"building with amenity", map[string]string{"building": "yes", "amenity": "hospital"}, models.CategoryHealthcare, true},
		{"building without qualifying tag", map[string]string{"building": "yes"}, "", false},
		{"highway excluded", map[string]string{"highway": "residential"}, "", false},
		{"natural excluded", map[string]string{"natural": "peak"}, "", false},
		{"empty tags excluded", map[string]string{}, "", false},
		{"unknown amenity excluded", map[string]string{"amenity": "bench"}, "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cat, ok := Evaluate(c.tags)
			if ok != c.included {
				t.Fatalf("inclusion mismatch: got %v want %v", ok, c.included)
			}
			if cat != c.want {
				t.Fatalf("category mismatch: got %q want %q", cat, c.want)
			}
		})
	}
}
