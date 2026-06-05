/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package identity

import (
	"context"
	"fmt"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// CandidateRepo is the only I/O surface Match touches.
type CandidateRepo interface {
	FindCandidates(ctx context.Context, lat, lng, radiusM float64, categories []models.Category) ([]models.Place, error)
}

// Match decides whether r refers to an existing place in the registry.
func Match(ctx context.Context, repo CandidateRepo, r Record) (Decision, error) {
	cats := Compatible(r.Category)
	candidates, err := repo.FindCandidates(ctx, r.Lat, r.Lng, RadiusM, cats)
	if err != nil {
		return Decision{}, fmt.Errorf("find candidates: %w", err)
	}
	if len(candidates) == 0 {
		return Decision{Kind: KindNoMatch}, nil
	}

	bestIdx := -1
	bestScore := -1.0
	for i, c := range candidates {
		score := scoreCandidate(r, c)
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	switch {
	case bestScore >= ConfidentThreshold:
		return Decision{Kind: KindConfident, MatchedPlaceID: candidates[bestIdx].ID, Confidence: bestScore}, nil
	case bestScore >= LowConfidenceThreshold:
		return Decision{Kind: KindLowConfidence, MatchedPlaceID: candidates[bestIdx].ID, Confidence: bestScore}, nil
	default:
		return Decision{Kind: KindNoMatch}, nil
	}
}

// scoreCandidate combines the three score components for a single candidate.
// Address lookups come from OSM tags addr:street and addr:housenumber.
func scoreCandidate(r Record, p models.Place) float64 {
	dist := distanceScore(r.Lat, r.Lng, p.Lat, p.Lng)
	name := nameScore(r.Name, p.Name)
	pStreet := p.Tags["addr:street"]
	pHouse := p.Tags["addr:housenumber"]
	addr, present := addressScore(r.Street, r.HouseNumber, pStreet, pHouse)
	return combinedScore(dist, name, addr, present)
}
