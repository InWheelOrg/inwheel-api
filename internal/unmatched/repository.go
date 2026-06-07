/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

// Package unmatched owns reads and writes against the unmatched_external table.
package unmatched

import (
	"context"
	"fmt"
	"time"

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

// FindCandidatesNearTouched returns queue rows whose geom is within radiusM
// metres of any place in touchedIDs. DISTINCT ensures a row near several
// touched places appears once.
func (r *Repository) FindCandidatesNearTouched(
	ctx context.Context,
	touchedIDs []string,
	radiusM float64,
) ([]models.UnmatchedExternal, error) {
	if len(touchedIDs) == 0 {
		return nil, nil
	}
	var out []models.UnmatchedExternal
	tx := r.db.WithContext(ctx).Raw(
		`SELECT DISTINCT u.*
		 FROM unmatched_external u
		 JOIN places p ON ST_DWithin(u.geom, geography(ST_Point(p.lng, p.lat)), ?)
		 WHERE p.id IN (?)`,
		radiusM, touchedIDs,
	).Scan(&out)
	if tx.Error != nil {
		return nil, fmt.Errorf("find candidates near touched: %w", tx.Error)
	}
	return out, nil
}

// BumpAttempts increments attempts and updates last_attempted on the row.
func (r *Repository) BumpAttempts(ctx context.Context, queueID int64, lastAttempted time.Time) error {
	tx := r.db.WithContext(ctx).Exec(
		`UPDATE unmatched_external
		 SET attempts = attempts + 1,
		     last_attempted = ?
		 WHERE id = ?`,
		lastAttempted, queueID,
	)
	if tx.Error != nil {
		return fmt.Errorf("bump attempts: %w", tx.Error)
	}
	if tx.RowsAffected == 0 {
		return fmt.Errorf("bump attempts: queue row %d not found", queueID)
	}
	return nil
}

// Delete removes the queue row by id.
func (r *Repository) Delete(ctx context.Context, queueID int64) error {
	tx := r.db.WithContext(ctx).Exec(
		`DELETE FROM unmatched_external WHERE id = ?`,
		queueID,
	)
	if tx.Error != nil {
		return fmt.Errorf("delete unmatched: %w", tx.Error)
	}
	if tx.RowsAffected == 0 {
		return fmt.Errorf("delete unmatched: queue row %d not found", queueID)
	}
	return nil
}

// Compile-time assertions that *Repository satisfies both queue interfaces.
var (
	_ identity.EnqueueRepo = (*Repository)(nil)
	_ identity.SweepRepo   = (*Repository)(nil)
)
