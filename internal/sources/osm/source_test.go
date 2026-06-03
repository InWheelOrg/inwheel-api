/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package osm

import (
	"context"
	"errors"
	"testing"

	"github.com/InWheelOrg/inwheel-api/internal/sources"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

func TestSource_Name(t *testing.T) {
	s := &Source{PBFPath: "irrelevant"}
	if got := s.Name(); got != "osm" {
		t.Errorf("Name() = %q, want %q", got, "osm")
	}
}

func TestSource_ImplementsFullImporter(t *testing.T) {
	var _ sources.FullImporter = (*Source)(nil)
}

func TestSource_FullImport_OpenError(t *testing.T) {
	s := &Source{PBFPath: "/no/such/file.pbf"}
	err := s.FullImport(context.Background(), func(context.Context, models.Place) error { return nil })
	if err == nil {
		t.Fatal("expected error opening missing file, got nil")
	}
}

func TestSource_FullImport_EmitsFromFixture(t *testing.T) {
	s := &Source{PBFPath: "../../../testdata/andorra-sample.osm.pbf"}

	var emitted int
	sink := func(_ context.Context, _ models.Place) error {
		emitted++
		return nil
	}

	if err := s.FullImport(context.Background(), sink); err != nil {
		t.Fatalf("FullImport: %v", err)
	}
	if emitted == 0 {
		t.Fatal("expected at least one emitted place, got zero")
	}
	t.Logf("emitted %d places from Andorra fixture", emitted)
}

func TestSource_FullImport_PropagatesSinkError(t *testing.T) {
	s := &Source{PBFPath: "../../../testdata/andorra-sample.osm.pbf"}

	sentinel := errors.New("sink stop")
	sink := func(_ context.Context, _ models.Place) error {
		return sentinel
	}

	err := s.FullImport(context.Background(), sink)
	if err == nil {
		t.Fatal("expected sink error to propagate, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected error to wrap sentinel, got %v", err)
	}
}
