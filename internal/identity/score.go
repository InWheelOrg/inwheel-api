/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package identity

import (
	"math"
	"strings"
)

// Tuning constants. These shape every matching outcome.
const (
	// RadiusM is the candidate fetch radius in metres and the normaliser for
	// distanceScore.
	RadiusM = 50.0

	// ConfidentThreshold is the minimum combined score to attach an external
	// reference without a low-confidence flag.
	ConfidentThreshold = 0.80

	// LowConfidenceThreshold is the minimum combined score to attach at all.
	LowConfidenceThreshold = 0.55

	// WeightDistance is the weight applied to distanceScore in combinedScore.
	WeightDistance = 0.5
	// WeightName is the weight applied to nameScore in combinedScore.
	WeightName = 0.4
	// WeightAddress is the weight applied to addressScore in combinedScore.
	WeightAddress = 0.1
)

// distanceScore returns 1.0 at coincident points and 0.0 at RadiusM, with a
// linear falloff in between. Clamped to 0 for distances beyond RadiusM.
func distanceScore(rLat, rLng, pLat, pLng float64) float64 {
	meters := haversineMeters(rLat, rLng, pLat, pLng)
	if meters >= RadiusM {
		return 0
	}
	return 1 - meters/RadiusM
}

// haversineMeters returns the great-circle distance between two WGS-84 points.
func haversineMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusM = 6371008.8
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLng := toRad(lng2 - lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusM * c
}

// nameScore returns the token-set Jaccard similarity of the two normalized
// names. Returns 0 when either side normalizes to an empty token set.
func nameScore(rName, pName string) float64 {
	r := tokenSet(Normalize(rName))
	p := tokenSet(Normalize(pName))
	if len(r) == 0 || len(p) == 0 {
		return 0
	}
	var intersect int
	for tok := range r {
		if _, ok := p[tok]; ok {
			intersect++
		}
	}
	union := len(r) + len(p) - intersect
	if union == 0 {
		return 0
	}
	return float64(intersect) / float64(union)
}

func tokenSet(toks []string) map[string]struct{} {
	set := make(map[string]struct{}, len(toks))
	for _, t := range toks {
		set[t] = struct{}{}
	}
	return set
}

// addressScore reports a 0/0.5/1 match score and a present flag. present is
// false when either side has no street.
func addressScore(rStreet, rHouse, pStreet, pHouse string) (score float64, present bool) {
	if rStreet == "" || pStreet == "" {
		return 0, false
	}
	if !streetsMatch(rStreet, pStreet) {
		return 0, true
	}
	if rHouse != "" && pHouse != "" && strings.EqualFold(rHouse, pHouse) {
		return 1.0, true
	}
	return 0.5, true
}

func streetsMatch(a, b string) bool {
	at := Normalize(a)
	bt := Normalize(b)
	if len(at) != len(bt) {
		return false
	}
	for i := range at {
		if at[i] != bt[i] {
			return false
		}
	}
	return true
}

// combinedScore folds the three component scores into a single value in [0, 1].
// When addrPresent is false, WeightAddress is redistributed between distance
// and name in proportion to their existing weights.
func combinedScore(distance, name, address float64, addrPresent bool) float64 {
	if addrPresent {
		return WeightDistance*distance + WeightName*name + WeightAddress*address
	}
	denom := WeightDistance + WeightName
	wDist := WeightDistance + WeightAddress*WeightDistance/denom
	wName := WeightName + WeightAddress*WeightName/denom
	return wDist*distance + wName*name
}
