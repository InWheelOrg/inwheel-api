/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package osm

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/paulmach/orb"
)

func TestStreamElements_FixturePBF(t *testing.T) {
	t.Parallel()
	f, err := os.Open("../../../testdata/andorra-sample.osm.pbf")
	if err != nil {
		t.Skipf("fixture PBF not available: %v", err)
	}
	defer f.Close() //nolint:errcheck

	ctx := context.Background()
	var totalNodes, includedPois int
	err = StreamElements(ctx, f, func(node Node) error {
		totalNodes++
		if _, ok := Evaluate(node.Tags); ok {
			includedPois++
		}
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("StreamElements returned an error: %v", err)
	}

	if totalNodes == 0 {
		t.Fatal("expected at least one node from the fixture PBF, got zero")
	}
	t.Logf("streamed %d total nodes, %d qualifying POIs", totalNodes, includedPois)
}

func TestStreamElements_VisitsWays(t *testing.T) {
	t.Parallel()
	f, err := os.Open("../../../testdata/andorra-sample.osm.pbf")
	if err != nil {
		t.Skipf("fixture PBF not available: %v", err)
	}
	defer f.Close() //nolint:errcheck

	ctx := context.Background()
	var totalWays, resolvedWays int
	err = StreamElements(ctx, f, func(Node) error { return nil }, func(way Way) error {
		totalWays++
		if way.Lat != 0 || way.Lng != 0 {
			resolvedWays++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("StreamElements returned an error: %v", err)
	}
	if totalWays == 0 {
		t.Fatal("expected at least one way from the fixture PBF, got zero")
	}
	if resolvedWays == 0 {
		t.Fatal("expected at least one way with a resolved centroid (annotated node locations), got zero")
	}
}

func TestCentroid_MeanOfPoints(t *testing.T) {
	t.Parallel()
	ls := orb.LineString{
		{6.0, 46.0},
		{8.0, 48.0},
	}
	lat, lng := centroid(ls)
	if lat != 47.0 {
		t.Errorf("lat: got %v want 47.0", lat)
	}
	if lng != 7.0 {
		t.Errorf("lng: got %v want 7.0", lng)
	}
}

func TestStreamElements_StopsOnSinkError(t *testing.T) {
	t.Parallel()
	f, err := os.Open("../../../testdata/andorra-sample.osm.pbf")
	if err != nil {
		t.Skipf("fixture PBF not available: %v", err)
	}
	defer f.Close() //nolint:errcheck

	ctx := context.Background()
	sentinel := errors.New("stop")
	count := 0
	err = StreamElements(ctx, f, func(node Node) error {
		count++
		return sentinel
	}, nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if count != 1 {
		t.Errorf("expected stream to stop after first node, got %d nodes", count)
	}
}
