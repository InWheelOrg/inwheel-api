/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package osm

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/paulmach/orb"
	pmosm "github.com/paulmach/osm"
	"github.com/paulmach/osm/osmpbf"
)

// Node is a domain representation of an OSM node, decoupled from the parser library.
type Node struct {
	ID   int64
	Lat  float64
	Lng  float64
	Tags map[string]string
}

// NodeSink is the callback invoked for each node read from a PBF stream.
// Returning a non-nil error stops the stream and propagates the error to the caller.
type NodeSink func(Node) error

// Way is a domain representation of an OSM way. Lat/Lng is the centroid of its resolved
// nodes, not a stored polygon.
type Way struct {
	ID   int64
	Lat  float64
	Lng  float64
	Tags map[string]string
}

// WaySink is the callback invoked for each way read from a PBF stream, sibling to NodeSink.
type WaySink func(Way) error

// StreamElements reads a .osm.pbf stream and invokes nodeSink per node and waySink per way
// with resolved node locations. A nil waySink skips ways silently. Relations are always skipped.
func StreamElements(ctx context.Context, r io.Reader, nodeSink NodeSink, waySink WaySink) error {
	scanner := osmpbf.New(ctx, r, 1)
	defer scanner.Close() //nolint:errcheck

	for scanner.Scan() {
		obj := scanner.Object()
		switch o := obj.(type) {
		case *pmosm.Node:
			domainNode := Node{
				ID:   int64(o.ID),
				Lat:  o.Lat,
				Lng:  o.Lon,
				Tags: o.Tags.Map(),
			}
			if err := nodeSink(domainNode); err != nil {
				return fmt.Errorf("node %d: %w", domainNode.ID, err)
			}
		case *pmosm.Way:
			if waySink == nil {
				continue
			}
			ls := o.LineString()
			if len(ls) == 0 { // way nodes weren't annotated with locations in the PBF
				slog.Warn("skipping way: no resolved node locations", "source", "osm", "way_id", o.ID)
				continue
			}
			lat, lng := centroid(ls)
			domainWay := Way{
				ID:   int64(o.ID),
				Lat:  lat,
				Lng:  lng,
				Tags: o.Tags.Map(),
			}
			if err := waySink(domainWay); err != nil {
				return fmt.Errorf("way %d: %w", domainWay.ID, err)
			}
		default:
			continue // skip relations
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("pbf scan: %w", err)
	}
	return nil
}

func centroid(ls orb.LineString) (lat, lng float64) {
	var sumLat, sumLng float64
	for _, p := range ls {
		sumLng += p.X()
		sumLat += p.Y()
	}
	n := float64(len(ls))
	return sumLat / n, sumLng / n
}
