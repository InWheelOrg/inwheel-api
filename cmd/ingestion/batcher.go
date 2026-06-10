/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"context"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// batcher buffers (place, profile) pairs, writes places first via flush, then
// writes profiles using the UUIDs returned by flush. Single-goroutine.
type batcher struct {
	size             int
	flush            func(context.Context, []models.Place) error
	writeProfile     func(context.Context, string, *models.AccessibilityProfile) (bool, error)
	downgradeProfile func(*models.AccessibilityProfile) int

	buffer             []models.Place
	pendingProfiles    []*models.AccessibilityProfile
	written            int
	touchedIDs         []string
	profilesWritten    int
	profilesDowngraded int
}

func (b *batcher) sink(ctx context.Context, p models.Place, profile *models.AccessibilityProfile) error {
	b.buffer = append(b.buffer, p)
	b.pendingProfiles = append(b.pendingProfiles, profile)
	if len(b.buffer) >= b.size {
		return b.flushNow(ctx)
	}
	return nil
}

func (b *batcher) flushNow(ctx context.Context) error {
	if len(b.buffer) == 0 {
		return nil
	}
	if err := b.flush(ctx, b.buffer); err != nil {
		return err
	}
	b.written += len(b.buffer)
	for i, p := range b.buffer {
		if p.ID == "" {
			continue
		}
		b.touchedIDs = append(b.touchedIDs, p.ID)
		profile := b.pendingProfiles[i]
		if profile == nil || b.writeProfile == nil {
			continue
		}
		if b.downgradeProfile != nil {
			b.profilesDowngraded += b.downgradeProfile(profile)
		}
		ok, err := b.writeProfile(ctx, p.ID, profile)
		if err != nil {
			return err
		}
		if ok {
			b.profilesWritten++
		}
	}
	b.buffer = b.buffer[:0]
	b.pendingProfiles = b.pendingProfiles[:0]
	return nil
}
