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

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/InWheelOrg/inwheel-api/internal/identity"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

var ErrPlaceNotFound = errors.New("place not found")
var ErrInvalidPatch = errors.New("invalid merge patch")

const pgUniqueViolation = "23505"

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}

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

type PreparePatch func(*models.AccessibilityProfile)

func (r *Repository) UpsertProfile(ctx context.Context, placeID string, rawPatch []byte, prepare PreparePatch) (result models.AccessibilityProfile, created bool, err error) {
	for attempt := 0; attempt < 2; attempt++ {
		result, created, err = r.upsertProfileAttempt(ctx, placeID, rawPatch, prepare)
		if err == nil || !isUniqueViolation(err) {
			return result, created, err
		}
	}
	return result, created, err
}

func (r *Repository) upsertProfileAttempt(ctx context.Context, placeID string, rawPatch []byte, prepare PreparePatch) (result models.AccessibilityProfile, created bool, err error) {
	now := time.Now()
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&models.Place{}, "id = ?", placeID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPlaceNotFound
			}
			return fmt.Errorf("upsert profile: check place: %w", err)
		}

		var existing models.AccessibilityProfile
		loadErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("place_id = ?", placeID).First(&existing).Error
		if loadErr != nil && !errors.Is(loadErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("upsert profile: load existing: %w", loadErr)
		}
		exists := !errors.Is(loadErr, gorm.ErrRecordNotFound)

		existingJSON := []byte("{}")
		if exists {
			b, marshalErr := json.Marshal(existing)
			if marshalErr != nil {
				return fmt.Errorf("upsert profile: marshal existing: %w", marshalErr)
			}
			existingJSON = b
		}
		mergedJSON, mergeErr := jsonpatch.MergePatch(existingJSON, rawPatch)
		if mergeErr != nil {
			return fmt.Errorf("%w: %v", ErrInvalidPatch, mergeErr)
		}
		var merged models.AccessibilityProfile
		if err := json.Unmarshal(mergedJSON, &merged); err != nil {
			return fmt.Errorf("upsert profile: unmarshal merged: %w", err)
		}

		if prepare != nil {
			prepare(&merged)
		}
		merged.UpdatedAt = now

		if !exists {
			merged.PlaceID = placeID
			created = true
			if err := tx.Create(&merged).Error; err != nil {
				return err
			}
			result = merged
			return nil
		}

		updates := map[string]any{
			"source_reports": merged.SourceReports,
			"entrance":       merged.Entrance,
			"pathways":       merged.Pathways,
			"restroom":       merged.Restroom,
			"parking":        merged.Parking,
			"elevator":       merged.Elevator,
			"updated_at":     now,
			"submitted_by":   merged.SubmittedBy,
			"submitted_at":   merged.SubmittedAt,
			"user_verified":  merged.UserVerified,
		}
		if err := tx.Model(&existing).Updates(updates).Error; err != nil {
			return err
		}
		result = merged
		return nil
	})
	return result, created, err
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
			"source_reports": profile.SourceReports,
			"entrance":       profile.Entrance,
			"pathways":       profile.Pathways,
			"restroom":       profile.Restroom,
			"parking":        profile.Parking,
			"elevator":       profile.Elevator,
			"updated_at":     now,
			"submitted_by":   nil,
			"submitted_at":   nil,
		}
		result := tx.Model(&existing).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		written = result.RowsAffected > 0
		return nil
	})
	return written, err
}

var _ interface {
	FindCandidates(ctx context.Context, lat, lng, radiusM float64, categories []models.Category) ([]models.Place, error)
} = (*Repository)(nil)

var _ identity.AttachRepo = (*Repository)(nil)
