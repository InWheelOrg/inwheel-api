/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"context"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// batcher buffers places and flushes in fixed-size batches via flush.
// It is single-goroutine; cmd/ingestion runs ingestion serially.
type batcher struct {
	size       int
	flush      func(context.Context, []models.Place) error
	buffer     []models.Place
	written    int
	touchedIDs []string
}

func (b *batcher) sink(ctx context.Context, p models.Place) error {
	b.buffer = append(b.buffer, p)
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
	for _, p := range b.buffer {
		if p.ID != "" {
			b.touchedIDs = append(b.touchedIDs, p.ID)
		}
	}
	b.buffer = b.buffer[:0]
	return nil
}
