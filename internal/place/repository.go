/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package place

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/InWheelOrg/inwheel-api/internal/identity"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) UpsertBatch(ctx context.Context, places []models.Place) error {
	if len(places) == 0 {
		return nil
	}

	// TargetWhere matches the partial index (WHERE osm_id <> 0) so zero-OSMID
	// test fixtures don't collide on the unique constraint.
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

func (r *Repository) FindCandidates(
	ctx context.Context,
	lat, lng, radiusM float64,
	categories []models.Category,
) ([]models.Place, error) {
	if len(categories) == 0 {
		return nil, nil
	}
	var out []models.Place
	tx := r.db.WithContext(ctx).
		Where("status = ?", models.PlaceStatusActive).
		Where("category IN ?", categories).
		Where("ST_DWithin(geography(ST_Point(lng, lat)), geography(ST_Point(?, ?)), ?)", lng, lat, radiusM).
		Clauses(clause.OrderBy{
			Expression: clause.Expr{
				SQL:  "ST_Distance(geography(ST_Point(lng, lat)), geography(ST_Point(?, ?))) ASC",
				Vars: []interface{}{lng, lat},
			},
		}).
		Limit(32).
		Find(&out)
	if tx.Error != nil {
		return nil, fmt.Errorf("find candidates: %w", tx.Error)
	}
	return out, nil
}

func (r *Repository) AttachExternalRef(
	ctx context.Context,
	placeID, source string,
	ref models.ExternalRef,
) error {
	refJSON, err := json.Marshal(ref)
	if err != nil {
		return fmt.Errorf("marshal external ref: %w", err)
	}
	tx := r.db.WithContext(ctx).Exec(
		`UPDATE places
         SET external_ids = jsonb_set(
                COALESCE(external_ids, '{}'::jsonb),
                ARRAY[?],
                ?::jsonb,
                true
            ),
            updated_at = NOW()
         WHERE id = ?`,
		source, string(refJSON), placeID,
	)
	if tx.Error != nil {
		return fmt.Errorf("attach external ref: %w", tx.Error)
	}
	if tx.RowsAffected == 0 {
		return fmt.Errorf("attach external ref: place %q not found", placeID)
	}
	return nil
}

// UpsertProfile creates or replaces the accessibility profile. Always overwrites — API write path.
// Returns created=true when a new row was inserted, false when an existing row was updated.
func (r *Repository) UpsertProfile(ctx context.Context, placeID string, profile *models.AccessibilityProfile) (created bool, err error) {
	if profile == nil {
		return false, fmt.Errorf("upsert profile: nil profile")
	}
	now := time.Now()
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.AccessibilityProfile
		loadErr := tx.Where("place_id = ?", placeID).First(&existing).Error
		if loadErr != nil && !errors.Is(loadErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("upsert profile: load existing: %w", loadErr)
		}
		if errors.Is(loadErr, gorm.ErrRecordNotFound) {
			profile.PlaceID = placeID
			profile.UpdatedAt = now
			created = true
			return tx.Create(profile).Error
		}
		updates := map[string]any{
			"overall_status": profile.OverallStatus,
			"components":     profile.Components,
			"updated_at":     now,
			"submitted_by":   profile.SubmittedBy,
			"submitted_at":   profile.SubmittedAt,
			"user_verified":  profile.UserVerified,
		}
		if err := tx.Model(&existing).Clauses(clause.Returning{}).Updates(updates).Error; err != nil {
			return err
		}
		*profile = existing
		return nil
	})
	return created, err
}

// UpsertProfileIngestion creates or updates the accessibility profile but skips
// rows where user_verified=true, preserving human-submitted corrections.
// Returns written=true when a row was actually written.
func (r *Repository) UpsertProfileIngestion(ctx context.Context, placeID string, profile *models.AccessibilityProfile) (written bool, err error) {
	if profile == nil {
		return false, fmt.Errorf("upsert profile ingestion: nil profile")
	}
	now := time.Now()
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.AccessibilityProfile
		loadErr := tx.Where("place_id = ?", placeID).First(&existing).Error
		if loadErr != nil && !errors.Is(loadErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("upsert profile ingestion: load existing: %w", loadErr)
		}
		if errors.Is(loadErr, gorm.ErrRecordNotFound) {
			profile.PlaceID = placeID
			profile.UpdatedAt = now
			profile.UserVerified = false
			if err := tx.Create(profile).Error; err != nil {
				return err
			}
			written = true
			return nil
		}
		if existing.UserVerified {
			return nil
		}
		updates := map[string]any{
			"overall_status": profile.OverallStatus,
			"components":     profile.Components,
			"updated_at":     now,
			"submitted_by":   nil,
			"submitted_at":   nil,
		}
		if err := tx.Model(&existing).Updates(updates).Error; err != nil {
			return err
		}
		written = true
		return nil
	})
	return written, err
}

var _ interface {
	FindCandidates(ctx context.Context, lat, lng, radiusM float64, categories []models.Category) ([]models.Place, error)
} = (*Repository)(nil)

var _ identity.AttachRepo = (*Repository)(nil)
