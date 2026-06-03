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
)

func TestStreamNodes_FixturePBF(t *testing.T) {
	f, err := os.Open("../../../testdata/andorra-sample.osm.pbf")
	if err != nil {
		t.Skipf("fixture PBF not available: %v", err)
	}
	defer f.Close() //nolint:errcheck

	ctx := context.Background()
	var totalNodes, includedPois int
	err = StreamNodes(ctx, f, func(node Node) error {
		totalNodes++
		if _, ok := Evaluate(node.Tags); ok {
			includedPois++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("StreamNodes returned an error: %v", err)
	}

	if totalNodes == 0 {
		t.Fatal("expected at least one node from the fixture PBF, got zero")
	}
	t.Logf("streamed %d total nodes, %d qualifying POIs", totalNodes, includedPois)
}

func TestStreamNodes_StopsOnSinkError(t *testing.T) {
	f, err := os.Open("../../../testdata/andorra-sample.osm.pbf")
	if err != nil {
		t.Skipf("fixture PBF not available: %v", err)
	}
	defer f.Close() //nolint:errcheck

	ctx := context.Background()
	sentinel := errors.New("stop")
	count := 0
	err = StreamNodes(ctx, f, func(node Node) error {
		count++
		return sentinel
	})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if count != 1 {
		t.Errorf("expected stream to stop after first node, got %d nodes", count)
	}
}
