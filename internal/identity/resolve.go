/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package identity

import (
	"context"
	"fmt"
	"time"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// AttachRepo writes an external reference onto an existing place.
type AttachRepo interface {
	AttachExternalRef(ctx context.Context, placeID, source string, ref models.ExternalRef) error
}

// EnqueueRepo persists a record to the retry queue.
type EnqueueRepo interface {
	Enqueue(ctx context.Context, u models.UnmatchedExternal) error
}

// Resolver runs Match on an incoming Record and applies the resulting Decision.
// Candidates, Places, and Unmatched are typically backed by the same place
// repository; the three interfaces stay separate so each call site declares
// only what it needs.
type Resolver struct {
	Candidates CandidateRepo
	Places     AttachRepo
	Unmatched  EnqueueRepo
	// Now returns the current time. Nil defaults to time.Now.
	Now func() time.Time
}

// Resolve runs Match on rec and applies the resulting Decision: confident or
// low-confidence matches attach an external ref to the matched place; no-match
// enqueues rec for the retry sweep. The Decision is returned alongside any
// error so the caller can update counters without re-matching.
func (r *Resolver) Resolve(ctx context.Context, rec Record) (Decision, error) {
	now := r.Now
	if now == nil {
		now = time.Now
	}
	d, err := Match(ctx, r.Candidates, rec)
	if err != nil {
		return Decision{}, fmt.Errorf("match: %w", err)
	}
	switch d.Kind {
	case KindConfident, KindLowConfidence:
		ref := models.ExternalRef{
			ID:         rec.SourceID,
			Confidence: d.Confidence,
			MatchedAt:  now(),
		}
		if err := r.Places.AttachExternalRef(ctx, d.MatchedPlaceID, rec.Source, ref); err != nil {
			return Decision{}, fmt.Errorf("attach external ref: %w", err)
		}
	case KindNoMatch:
		u := models.UnmatchedExternal{
			Source:        rec.Source,
			SourceID:      rec.SourceID,
			Lat:           rec.Lat,
			Lng:           rec.Lng,
			Payload:       rec.Payload,
			LastAttempted: now(),
		}
		if err := r.Unmatched.Enqueue(ctx, u); err != nil {
			return Decision{}, fmt.Errorf("enqueue unmatched: %w", err)
		}
	}
	return d, nil
}
