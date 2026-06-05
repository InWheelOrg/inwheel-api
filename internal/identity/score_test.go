/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package identity

import (
	"math"
	"testing"
)

const floatTolerance = 1e-6

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < floatTolerance
}

func TestDistanceScore(t *testing.T) {
	// At this latitude, ~0.000449 deg lat is ~50 m.
	const lat = 46.4628
	const lng = 6.8417

	cases := []struct {
		name         string
		pLat         float64
		pLng         float64
		wantMinScore float64
		wantMaxScore float64
	}{
		{"coincident", lat, lng, 0.999, 1.0},
		{"25 m north", lat + 0.000225, lng, 0.45, 0.55},
		{"50 m north (boundary)", lat + 0.000449, lng, 0.0, 0.05},
		{"100 m north (beyond radius)", lat + 0.000898, lng, 0.0, 0.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := distanceScore(lat, lng, tc.pLat, tc.pLng)
			if got < tc.wantMinScore || got > tc.wantMaxScore {
				t.Errorf("distanceScore = %v, want in [%v, %v]", got, tc.wantMinScore, tc.wantMaxScore)
			}
		})
	}
}

func TestNameScore(t *testing.T) {
	cases := []struct {
		name string
		r    string
		p    string
		want float64
	}{
		{"identical", "Café Pascal", "Cafe Pascal", 1.0},
		{"case + punctuation differ", "PASCAL CAFE.", "cafe pascal", 1.0},
		{"one token overlap of two-each", "Cafe Pascal", "Cafe Roma", 1.0 / 3.0},
		{"no overlap", "Pascal", "Roma", 0.0},
		{"both empty after normalize", "", "", 0.0},
		{"one empty after normalize", "Pascal", "", 0.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nameScore(tc.r, tc.p)
			if !approxEqual(got, tc.want) {
				t.Errorf("nameScore(%q, %q) = %v, want %v", tc.r, tc.p, got, tc.want)
			}
		})
	}
}

func TestAddressScore(t *testing.T) {
	cases := []struct {
		name        string
		rStreet     string
		rHouse      string
		pStreet     string
		pHouse      string
		wantScore   float64
		wantPresent bool
	}{
		{"full match", "Rue du Simplon", "10", "Rue du Simplon", "10", 1.0, true},
		{"street match, both have house, house differs", "Rue du Simplon", "10", "Rue du Simplon", "12", 0.5, true},
		{"street match, only one side has house", "Rue du Simplon", "10", "Rue du Simplon", "", 0.5, true},
		{"street match, neither has house", "Rue du Simplon", "", "Rue du Simplon", "", 0.5, true},
		{"street mismatch", "Rue du Simplon", "10", "Grand-Rue", "10", 0.0, true},
		{"record has no street", "", "10", "Rue du Simplon", "10", 0.0, false},
		{"candidate has no street", "Rue du Simplon", "10", "", "10", 0.0, false},
		{"both have no street", "", "", "", "", 0.0, false},
		{"street match with diacritic difference", "Bulevárdi", "1", "Bulevardi", "1", 1.0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotScore, gotPresent := addressScore(tc.rStreet, tc.rHouse, tc.pStreet, tc.pHouse)
			if gotPresent != tc.wantPresent {
				t.Errorf("present = %v, want %v", gotPresent, tc.wantPresent)
			}
			if !approxEqual(gotScore, tc.wantScore) {
				t.Errorf("score = %v, want %v", gotScore, tc.wantScore)
			}
		})
	}
}

func TestCombinedScore(t *testing.T) {
	cases := []struct {
		name        string
		distance    float64
		nameVal     float64
		address     float64
		addrPresent bool
		want        float64
	}{
		{"all max, address present", 1.0, 1.0, 1.0, true, 1.0},
		{"all zero", 0.0, 0.0, 0.0, true, 0.0},
		{"address present weighted sum", 0.8, 0.6, 0.5, true, 0.5*0.8 + 0.4*0.6 + 0.1*0.5},
		{"address absent, all max distance and name", 1.0, 1.0, 0.0, false, 1.0},
		{"address absent, max distance only", 1.0, 0.0, 0.0, false, 0.5 + 0.1*0.5/0.9},
		{"address absent, max name only", 0.0, 1.0, 0.0, false, 0.4 + 0.1*0.4/0.9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := combinedScore(tc.distance, tc.nameVal, tc.address, tc.addrPresent)
			if !approxEqual(got, tc.want) {
				t.Errorf("combinedScore = %v, want %v", got, tc.want)
			}
		})
	}
}
