/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package osm

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/InWheelOrg/inwheel-api/internal/sources"
)

// Source streams a .osm.pbf file and emits matched POIs as models.Place
// records. It implements sources.FullImporter.
type Source struct {
	PBFPath string
}

func (s *Source) Name() string { return "osm" }

func (s *Source) Kind() sources.SourceKind { return sources.SourceKindCanonical }

func (s *Source) FullImport(ctx context.Context, sink sources.Sink) error {
	f, err := os.Open(s.PBFPath)
	if err != nil {
		return fmt.Errorf("open pbf: %w", err)
	}
	defer f.Close() //nolint:errcheck

	var processed, emitted, skipped int

	err = StreamElements(ctx, f, func(node Node) error {
		processed++
		if processed%10000 == 0 {
			slog.Info("source progress",
				"source", "osm",
				"processed", processed,
				"emitted", emitted,
			)
		}

		category, ok := Evaluate(node.Tags)
		if !ok {
			return nil
		}

		p, profile, err := TransformOSMNode(node, category)
		if err != nil {
			skipped++
			slog.Warn("skipping node",
				"source", "osm",
				"node_id", node.ID,
				"error", err,
			)
			return nil
		}

		emitted++
		return sink(ctx, *p, profile)
	}, func(way Way) error {
		processed++

		category, ok := Evaluate(way.Tags)
		if !ok {
			return nil
		}

		p, profile, err := TransformOSMWay(way, category)
		if err != nil {
			skipped++
			slog.Warn("skipping way",
				"source", "osm",
				"way_id", way.ID,
				"error", err,
			)
			return nil
		}

		emitted++
		return sink(ctx, *p, profile)
	})
	if err != nil {
		return fmt.Errorf("stream: %w", err)
	}

	slog.Info("source complete",
		"source", "osm",
		"processed", processed,
		"emitted", emitted,
		"skipped", skipped,
	)
	return nil
}
