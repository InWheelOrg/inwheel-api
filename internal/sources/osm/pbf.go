/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package osm

import (
	"context"
	"fmt"
	"io"

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
// Returning a non-nil error stops the stream and propagates the error to StreamNodes' caller.
type NodeSink func(Node) error

// StreamNodes reads a .osm.pbf stream and invokes sink for each node element.
// Ways and relations are skipped silently.
func StreamNodes(ctx context.Context, r io.Reader, sink NodeSink) error {
	scanner := osmpbf.New(ctx, r, 1)
	defer scanner.Close() //nolint:errcheck

	for scanner.Scan() {
		obj := scanner.Object()
		node, ok := obj.(*pmosm.Node)
		if !ok {
			continue // skip ways, relations
		}

		domainNode := Node{
			ID:   int64(node.ID),
			Lat:  node.Lat,
			Lng:  node.Lon,
			Tags: node.Tags.Map(),
		}

		if err := sink(domainNode); err != nil {
			return fmt.Errorf("node %d: %w", domainNode.ID, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("pbf scan: %w", err)
	}
	return nil
}
