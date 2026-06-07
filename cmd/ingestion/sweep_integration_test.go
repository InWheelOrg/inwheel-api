//go:build integration

/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/InWheelOrg/inwheel-api/internal/testhelpers"
	"github.com/InWheelOrg/inwheel-api/internal/unmatched"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

func TestRetrySweep_DrainsMatchableQueueRowsAfterOSMIngest(t *testing.T) {
	ctx := context.Background()
	db, connInfo, cleanup, err := testhelpers.StartPostgresWithConnInfo(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	queueRepo := unmatched.NewRepository(db)
	clock := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	confidentQueue := models.UnmatchedExternal{
		Source: "wheelmap", SourceID: "confident-target",
		Name: "Supermercat Saint Moritz", Category: "shop",
		Street: "Carretera d'Arinsal", HouseNumber: "16",
		Lat: 42.571637, Lng: 1.484545,
		Attempts: 1, LastAttempted: clock,
		Payload: json.RawMessage(`{"why":"confident"}`),
	}
	noMatchQueue := models.UnmatchedExternal{
		Source: "wheelmap", SourceID: "no-match-target",
		Name: "Pharmacy Phantom", Category: "healthcare",
		Lat: 42.571640, Lng: 1.484550,
		Attempts: 1, LastAttempted: clock,
		Payload: json.RawMessage(`{"why":"no-match"}`),
	}
	untouchedQueue := models.UnmatchedExternal{
		Source: "wheelmap", SourceID: "untouched-target",
		Name: "Pacific Phantom", Category: "cafe",
		Lat: 0.0, Lng: -150.0,
		Attempts: 3, LastAttempted: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Payload: json.RawMessage(`{"why":"untouched"}`),
	}
	for _, u := range []models.UnmatchedExternal{confidentQueue, noMatchQueue, untouchedQueue} {
		if err := queueRepo.Enqueue(ctx, u); err != nil {
			t.Fatalf("Enqueue %s: %v", u.SourceID, err)
		}
	}

	cfg := config{
		DBHost:     connInfo.Host,
		DBPort:     connInfo.Port,
		DBUser:     connInfo.User,
		DBPassword: connInfo.Password,
		DBName:     connInfo.Name,
		DBSSLMode:  connInfo.SSLMode,
		OSMPBFPath: fixturePBFPath,
	}
	if err := run(ctx, "osm", "full-import", cfg); err != nil {
		t.Fatalf("OSM run: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}

	t.Run("confident row deleted and external ref attached to matched place", func(t *testing.T) {
		var count int
		if err := sqlDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM unmatched_external WHERE source = $1 AND source_id = $2",
			"wheelmap", "confident-target",
		).Scan(&count); err != nil {
			t.Fatalf("count confident: %v", err)
		}
		if count != 0 {
			t.Errorf("confident queue row not deleted (count=%d)", count)
		}

		var place models.Place
		if err := db.Where("osm_id = ? AND osm_type = ?", int64(521143390), models.OSMNode).First(&place).Error; err != nil {
			t.Fatalf("lookup place: %v", err)
		}
		ref, ok := place.ExternalIDs["wheelmap"]
		if !ok {
			t.Fatalf("Supermercat external_ids has no wheelmap key: %#v", place.ExternalIDs)
		}
		if ref.ID != "confident-target" {
			t.Errorf("ref.ID = %q, want %q", ref.ID, "confident-target")
		}
		if ref.Confidence < 0.80 {
			t.Errorf("ref.Confidence = %v, want >= 0.80", ref.Confidence)
		}
	})

	t.Run("no-match row's attempts bumped, last_attempted updated", func(t *testing.T) {
		var attempts int
		var lastAttempted time.Time
		if err := sqlDB.QueryRowContext(ctx,
			"SELECT attempts, last_attempted FROM unmatched_external WHERE source = $1 AND source_id = $2",
			"wheelmap", "no-match-target",
		).Scan(&attempts, &lastAttempted); err != nil {
			t.Fatalf("scan no-match: %v", err)
		}
		if attempts != 2 {
			t.Errorf("no-match attempts = %d, want 2", attempts)
		}
		if !lastAttempted.After(clock) {
			t.Errorf("no-match last_attempted = %v, want after %v", lastAttempted, clock)
		}
	})

	t.Run("untouched-region row unchanged", func(t *testing.T) {
		var attempts int
		var lastAttempted time.Time
		if err := sqlDB.QueryRowContext(ctx,
			"SELECT attempts, last_attempted FROM unmatched_external WHERE source = $1 AND source_id = $2",
			"wheelmap", "untouched-target",
		).Scan(&attempts, &lastAttempted); err != nil {
			t.Fatalf("scan untouched: %v", err)
		}
		if attempts != 3 {
			t.Errorf("untouched attempts = %d, want 3 (unchanged)", attempts)
		}
		wantTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		if lastAttempted.UnixMicro() != wantTime.UnixMicro() {
			t.Errorf("untouched last_attempted = %v, want %v (unchanged)", lastAttempted, wantTime)
		}
	})
}
