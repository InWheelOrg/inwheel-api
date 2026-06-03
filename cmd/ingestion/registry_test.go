/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"strings"
	"testing"

	"github.com/InWheelOrg/inwheel-api/internal/sources/osm"
)

func TestBuildSource_OSMHappyPath(t *testing.T) {
	cfg := config{OSMPBFPath: "/tmp/x.pbf"}
	src, err := buildSource("osm", cfg)
	if err != nil {
		t.Fatalf("buildSource: %v", err)
	}
	got, ok := src.(*osm.Source)
	if !ok {
		t.Fatalf("expected *osm.Source, got %T", src)
	}
	if got.PBFPath != "/tmp/x.pbf" {
		t.Errorf("PBFPath = %q, want %q", got.PBFPath, "/tmp/x.pbf")
	}
}

func TestBuildSource_OSMMissingConfig(t *testing.T) {
	_, err := buildSource("osm", config{})
	if err == nil {
		t.Fatal("expected error for missing OSM_PBF_PATH, got nil")
	}
	if !strings.Contains(err.Error(), "OSM_PBF_PATH") {
		t.Errorf("error should mention OSM_PBF_PATH, got %v", err)
	}
}

func TestBuildSource_UnknownSource(t *testing.T) {
	_, err := buildSource("wheelmap", config{})
	if err == nil {
		t.Fatal("expected error for unknown source, got nil")
	}
	if !strings.Contains(err.Error(), "wheelmap") {
		t.Errorf("error should name the source, got %v", err)
	}
}
