/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

func TestBatcher_FlushesWhenFull(t *testing.T) {
	var flushes [][]models.Place
	b := &batcher{
		size: 2,
		flush: func(_ context.Context, batch []models.Place) error {
			cp := make([]models.Place, len(batch))
			copy(cp, batch)
			flushes = append(flushes, cp)
			return nil
		},
	}

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := b.sink(ctx, models.Place{}, nil); err != nil {
			t.Fatalf("sink %d: %v", i, err)
		}
	}

	if len(flushes) != 1 {
		t.Fatalf("expected 1 size-triggered flush, got %d", len(flushes))
	}
	if len(flushes[0]) != 2 {
		t.Errorf("expected first flush size 2, got %d", len(flushes[0]))
	}
	if b.written != 2 {
		t.Errorf("written = %d, want 2", b.written)
	}
}

func TestBatcher_FlushNow_DrainsPartialBuffer(t *testing.T) {
	var flushes [][]models.Place
	b := &batcher{
		size: 10,
		flush: func(_ context.Context, batch []models.Place) error {
			cp := make([]models.Place, len(batch))
			copy(cp, batch)
			flushes = append(flushes, cp)
			return nil
		},
	}

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := b.sink(ctx, models.Place{}, nil); err != nil {
			t.Fatalf("sink: %v", err)
		}
	}
	if len(flushes) != 0 {
		t.Fatalf("expected no flushes before flushNow, got %d", len(flushes))
	}

	if err := b.flushNow(ctx); err != nil {
		t.Fatalf("flushNow: %v", err)
	}
	if len(flushes) != 1 || len(flushes[0]) != 3 {
		t.Fatalf("expected one flush of size 3, got %v", flushes)
	}
	if b.written != 3 {
		t.Errorf("written = %d, want 3", b.written)
	}
}

func TestBatcher_FlushNow_NoOpWhenEmpty(t *testing.T) {
	called := false
	b := &batcher{
		size: 10,
		flush: func(context.Context, []models.Place) error {
			called = true
			return nil
		},
	}
	if err := b.flushNow(context.Background()); err != nil {
		t.Fatalf("flushNow: %v", err)
	}
	if called {
		t.Error("flush should not be called for empty buffer")
	}
}

func TestBatcher_PropagatesFlushError(t *testing.T) {
	sentinel := errors.New("boom")
	b := &batcher{
		size: 1,
		flush: func(context.Context, []models.Place) error {
			return sentinel
		},
	}
	err := b.sink(context.Background(), models.Place{}, nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel, got %v", err)
	}
}

func TestBatcher_TouchedIDsAccumulateAcrossFlushes(t *testing.T) {
	flushed := [][]models.Place{}
	b := &batcher{
		size: 2,
		flush: func(_ context.Context, ps []models.Place) error {
			for i := range ps {
				ps[i].ID = fmt.Sprintf("id-%d-%d", len(flushed), i)
			}
			cp := make([]models.Place, len(ps))
			copy(cp, ps)
			flushed = append(flushed, cp)
			return nil
		},
	}
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := b.sink(ctx, models.Place{Name: fmt.Sprintf("p%d", i)}, nil); err != nil {
			t.Fatalf("sink: %v", err)
		}
	}
	if err := b.flushNow(ctx); err != nil {
		t.Fatalf("flushNow: %v", err)
	}
	if got := len(b.touchedIDs); got != 5 {
		t.Errorf("touchedIDs length = %d, want 5", got)
	}
	wantIDs := map[string]bool{
		"id-0-0": true, "id-0-1": true,
		"id-1-0": true, "id-1-1": true,
		"id-2-0": true,
	}
	for _, id := range b.touchedIDs {
		if !wantIDs[id] {
			t.Errorf("unexpected id in touchedIDs: %q", id)
		}
	}
}

func TestBatcher_WritesProfileWhenAttached(t *testing.T) {
	ctx := context.Background()
	var captured []capturedProfile
	b := &batcher{
		size: 10,
		flush: func(_ context.Context, ps []models.Place) error {
			for i := range ps {
				ps[i].ID = fmt.Sprintf("place-%d", i)
			}
			return nil
		},
		writeProfile: func(_ context.Context, placeID string, p *models.AccessibilityProfile) (bool, error) {
			captured = append(captured, capturedProfile{placeID: placeID, profile: p})
			return true, nil
		},
		downgradeProfile: func(_ *models.AccessibilityProfile) int { return 0 },
	}

	if err := b.sink(ctx, models.Place{Name: "no-a11y"}, nil); err != nil {
		t.Fatalf("sink p1: %v", err)
	}
	profile := &models.AccessibilityProfile{OverallStatus: models.StatusAccessible}
	if err := b.sink(ctx, models.Place{Name: "with-a11y"}, profile); err != nil {
		t.Fatalf("sink p2: %v", err)
	}
	if err := b.flushNow(ctx); err != nil {
		t.Fatalf("flushNow: %v", err)
	}
	if len(captured) != 1 {
		t.Fatalf("captured = %d, want 1 profile write", len(captured))
	}
	if captured[0].placeID != "place-1" {
		t.Errorf("placeID = %q, want place-1", captured[0].placeID)
	}
	if b.profilesWritten != 1 {
		t.Errorf("profilesWritten = %d, want 1", b.profilesWritten)
	}
}

func TestBatcher_DowngradeCounter(t *testing.T) {
	ctx := context.Background()
	b := &batcher{
		size: 10,
		flush: func(_ context.Context, ps []models.Place) error {
			for i := range ps {
				ps[i].ID = fmt.Sprintf("place-%d", i)
			}
			return nil
		},
		writeProfile: func(_ context.Context, _ string, _ *models.AccessibilityProfile) (bool, error) {
			return true, nil
		},
		downgradeProfile: func(_ *models.AccessibilityProfile) int { return 2 },
	}
	profile := &models.AccessibilityProfile{OverallStatus: models.StatusAccessible}
	if err := b.sink(ctx, models.Place{Name: "p"}, profile); err != nil {
		t.Fatalf("sink: %v", err)
	}
	if err := b.flushNow(ctx); err != nil {
		t.Fatalf("flushNow: %v", err)
	}
	if b.profilesDowngraded != 2 {
		t.Errorf("profilesDowngraded = %d, want 2", b.profilesDowngraded)
	}
}

func TestBatcher_PlaceHasNoAccessibilityFieldInFlush(t *testing.T) {
	ctx := context.Background()
	var batchSeen []models.Place
	b := &batcher{
		size: 10,
		flush: func(_ context.Context, ps []models.Place) error {
			batchSeen = make([]models.Place, len(ps))
			copy(batchSeen, ps)
			for i := range ps {
				ps[i].ID = fmt.Sprintf("place-%d", i)
			}
			return nil
		},
		writeProfile: func(_ context.Context, _ string, _ *models.AccessibilityProfile) (bool, error) {
			return true, nil
		},
		downgradeProfile: func(_ *models.AccessibilityProfile) int { return 0 },
	}
	profile := &models.AccessibilityProfile{OverallStatus: models.StatusAccessible}
	if err := b.sink(ctx, models.Place{Name: "with-a11y"}, profile); err != nil {
		t.Fatalf("sink: %v", err)
	}
	if err := b.flushNow(ctx); err != nil {
		t.Fatalf("flushNow: %v", err)
	}
	if len(batchSeen) != 1 {
		t.Fatalf("expected 1 place in batch, got %d", len(batchSeen))
	}
	if batchSeen[0].Accessibility != nil {
		t.Errorf("place.Accessibility must be nil in flush batch, got %+v", batchSeen[0].Accessibility)
	}
}

type capturedProfile struct {
	placeID string
	profile *models.AccessibilityProfile
}
