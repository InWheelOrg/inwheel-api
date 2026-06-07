/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package identity

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// SweepRepo is the queue-side surface Sweep needs: find candidates near touched
// places, delete a row, bump attempts on a row.
type SweepRepo interface {
	FindCandidatesNearTouched(ctx context.Context, touchedIDs []string, radiusM float64) ([]models.UnmatchedExternal, error)
	BumpAttempts(ctx context.Context, queueID int64, lastAttempted time.Time) error
	Delete(ctx context.Context, queueID int64) error
}

// SweepResult is the per-run summary returned alongside any fatal error.
type SweepResult struct {
	Considered    int
	Confident     int
	LowConfidence int
	NoMatch       int
	Errors        int
}

// Sweeper drains unmatched_external by re-running Match against the now-richer
// places table after every OSM ingest.
type Sweeper struct {
	Candidates CandidateRepo
	Places     AttachRepo
	Queue      SweepRepo
	Now        func() time.Time
}

// Sweep re-evaluates queue rows near the given touched place IDs. Per-row
// errors are logged and counted but do not abort the run. Returns a per-run
// summary plus a fatal error only if the initial find query fails.
func (s *Sweeper) Sweep(ctx context.Context, touchedIDs []string) (SweepResult, error) {
	if len(touchedIDs) == 0 {
		return SweepResult{}, nil
	}
	now := s.Now
	if now == nil {
		now = time.Now
	}
	rows, err := s.Queue.FindCandidatesNearTouched(ctx, touchedIDs, RadiusM)
	if err != nil {
		return SweepResult{}, fmt.Errorf("find candidates near touched: %w", err)
	}
	res := SweepResult{Considered: len(rows)}
	for _, row := range rows {
		rec := Record{
			Source:      row.Source,
			SourceID:    row.SourceID,
			Name:        row.Name,
			Lat:         row.Lat,
			Lng:         row.Lng,
			Category:    models.Category(row.Category),
			Street:      row.Street,
			HouseNumber: row.HouseNumber,
			Payload:     row.Payload,
		}
		d, err := Match(ctx, s.Candidates, rec)
		if err != nil {
			res.Errors++
			slog.Warn("sweep match failed", "queue_id", row.ID, "error", err)
			continue
		}
		switch d.Kind {
		case KindConfident, KindLowConfidence:
			ref := models.ExternalRef{
				ID:         rec.SourceID,
				Confidence: d.Confidence,
				MatchedAt:  now(),
			}
			if err := s.Places.AttachExternalRef(ctx, d.MatchedPlaceID, rec.Source, ref); err != nil {
				res.Errors++
				slog.Warn("sweep attach failed", "queue_id", row.ID, "error", err)
				continue
			}
			if err := s.Queue.Delete(ctx, row.ID); err != nil {
				res.Errors++
				slog.Warn("sweep delete failed", "queue_id", row.ID, "error", err)
				continue
			}
			if d.Kind == KindConfident {
				res.Confident++
			} else {
				res.LowConfidence++
			}
		case KindNoMatch:
			if err := s.Queue.BumpAttempts(ctx, row.ID, now()); err != nil {
				res.Errors++
				slog.Warn("sweep bump failed", "queue_id", row.ID, "error", err)
				continue
			}
			res.NoMatch++
		default:
			res.Errors++
			slog.Warn("sweep unknown decision kind", "queue_id", row.ID, "kind", d.Kind)
		}
	}
	return res, nil
}
