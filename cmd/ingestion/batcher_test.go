/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"context"
	"errors"
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
		if err := b.sink(ctx, models.Place{}); err != nil {
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
		if err := b.sink(ctx, models.Place{}); err != nil {
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
	err := b.sink(context.Background(), models.Place{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel, got %v", err)
	}
}
