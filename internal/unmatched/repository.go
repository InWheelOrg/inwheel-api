/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

// Package unmatched owns reads and writes against the unmatched_external table.
package unmatched

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/InWheelOrg/inwheel-api/internal/identity"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// Repository is the data-access layer for the unmatched_external queue table.
type Repository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository backed by the given GORM connection.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Enqueue inserts u into unmatched_external. On (source, source_id) conflict,
// bumps attempts and refreshes payload, last_attempted, matchable fields, and coordinates.
func (r *Repository) Enqueue(ctx context.Context, u models.UnmatchedExternal) error {
	tx := r.db.WithContext(ctx).Exec(
		`INSERT INTO unmatched_external
            (source, source_id, name, category, street, housenumber,
             lat, lng, geom, payload, last_attempted, attempts)
         VALUES
            (?, ?, ?, ?, ?, ?,
             ?, ?, ST_Point(?, ?)::geography, ?, ?, ?)
         ON CONFLICT (source, source_id) DO UPDATE SET
            attempts       = unmatched_external.attempts + 1,
            last_attempted = EXCLUDED.last_attempted,
            payload        = EXCLUDED.payload,
            name           = EXCLUDED.name,
            category       = EXCLUDED.category,
            street         = EXCLUDED.street,
            housenumber    = EXCLUDED.housenumber,
            lat            = EXCLUDED.lat,
            lng            = EXCLUDED.lng,
            geom           = EXCLUDED.geom`,
		u.Source, u.SourceID, u.Name, u.Category, u.Street, u.HouseNumber,
		u.Lat, u.Lng, u.Lng, u.Lat,
		u.Payload, u.LastAttempted, u.Attempts,
	)
	if tx.Error != nil {
		return fmt.Errorf("enqueue unmatched: %w", tx.Error)
	}
	return nil
}

// Compile-time assertion that *Repository satisfies identity.EnqueueRepo.
var _ identity.EnqueueRepo = (*Repository)(nil)
