/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package identity

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
	return 0
}

// nameScore returns the token-set Jaccard similarity of the two normalized
// names. Returns 0 when either side normalizes to an empty token set.
func nameScore(rName, pName string) float64 {
	return 0
}

// addressScore reports a 0/0.5/1 match score and a present flag. present is
// false when either side has no street.
func addressScore(rStreet, rHouse, pStreet, pHouse string) (score float64, present bool) {
	return 0, false
}

// combinedScore folds the three component scores into a single value in [0, 1].
// When addrPresent is false, WeightAddress is redistributed between distance
// and name in proportion to their existing weights.
func combinedScore(distance, name, address float64, addrPresent bool) float64 {
	return 0
}
