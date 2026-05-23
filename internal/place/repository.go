/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

// Package place owns reads and writes against the places table.
package place

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// Repository is the data-access layer for places.
type Repository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository backed by the given GORM connection.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// UpsertBatch inserts or updates the given places in a single SQL statement using
// (osm_id, osm_type) as the conflict key. Existing rows have their name, location,
// category, rank, tags, external_ids, and status replaced. Returns an error if the
// underlying SQL fails. An empty or nil batch is a no-op.
func (r *Repository) UpsertBatch(ctx context.Context, places []models.Place) error {
	if len(places) == 0 {
		return nil
	}

	// TargetWhere matches the partial index predicate. The index is defined
	// WHERE osm_id <> 0 so test fixtures that create places without setting
	// osm_id (zero value) don't collide on the unique constraint. In production,
	// every place is OSM-sourced and has a non-zero osm_id, so the predicate
	// covers every real row.
	tx := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "osm_id"},
			{Name: "osm_type"},
		},
		TargetWhere: clause.Where{Exprs: []clause.Expression{
			clause.Expr{SQL: "osm_id <> 0"},
		}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "lat", "lng", "category", "rank", "tags", "external_ids", "status", "updated_at",
		}),
	}).Create(&places)

	if tx.Error != nil {
		return fmt.Errorf("upsert places: %w", tx.Error)
	}
	return nil
}
