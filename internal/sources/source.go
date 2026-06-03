/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

// Package sources defines the abstraction for external place-data sources
// consumed by cmd/ingestion. Concrete sources live under internal/sources/<name>.
package sources

import (
	"context"
	"time"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// Sink receives one place at a time. The caller decides whether to batch
// downstream writes.
type Sink func(context.Context, models.Place) error

// Source identifies a place data source.
type Source interface {
	// Name returns a stable identifier such as "osm" or "wheelmap".
	Name() string
}

// FullImporter performs a complete pull from the source.
type FullImporter interface {
	Source
	FullImport(ctx context.Context, sink Sink) error
}

// DiffSyncer pulls changes from the source since the given timestamp.
type DiffSyncer interface {
	Source
	DiffSync(ctx context.Context, since time.Time, sink Sink) error
}
