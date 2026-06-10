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

	"github.com/InWheelOrg/inwheel-api/internal/identity"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// SourceKind discriminates canonical place-registry sources (OSM) from
// external accessibility sources (Wheelmap etc.) that attach to existing places.
type SourceKind int

const (
	// SourceKindCanonical sources own place rows.
	SourceKindCanonical SourceKind = iota
	// SourceKindExternal sources contribute external IDs and accessibility
	// data attached to existing places via identity.Match.
	SourceKindExternal
)

// Sink receives one place and its optional accessibility profile. Used by canonical sources.
type Sink func(context.Context, models.Place, *models.AccessibilityProfile) error

// RecordSink receives one identity.Record at a time. Used by external sources.
type RecordSink func(context.Context, identity.Record) error

// Source identifies a place data source.
type Source interface {
	// Name returns a stable identifier such as "osm" or "wheelmap".
	Name() string
	// Kind returns the pipeline this source belongs to.
	Kind() SourceKind
}

// FullImporter performs a complete pull from a canonical source.
type FullImporter interface {
	Source
	FullImport(ctx context.Context, sink Sink) error
}

// DiffSyncer pulls changes from a canonical source since the given timestamp.
type DiffSyncer interface {
	Source
	DiffSync(ctx context.Context, since time.Time, sink Sink) error
}

// ExternalFullImporter performs a complete pull from an external source.
type ExternalFullImporter interface {
	Source
	FullImport(ctx context.Context, sink RecordSink) error
}

// ExternalDiffSyncer pulls changes from an external source since the given timestamp.
type ExternalDiffSyncer interface {
	Source
	DiffSync(ctx context.Context, since time.Time, sink RecordSink) error
}
