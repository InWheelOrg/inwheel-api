/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package identity

import (
	"context"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// CandidateRepo is the only I/O surface Match touches: a spatial + category
// query against the places table.
type CandidateRepo interface {
	// FindCandidates returns active places within radiusM metres of (lat, lng)
	// whose category is in categories.
	FindCandidates(ctx context.Context, lat, lng, radiusM float64, categories []models.Category) ([]models.Place, error)
}

// Match decides whether r refers to an existing place in the registry.
// Returns a Decision describing the outcome and a non-nil error only when
// the repository call fails.
func Match(ctx context.Context, repo CandidateRepo, r Record) (Decision, error) {
	return Decision{}, nil
}
