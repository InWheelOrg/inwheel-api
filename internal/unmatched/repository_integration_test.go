//go:build integration

/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package unmatched_test

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/InWheelOrg/inwheel-api/internal/testhelpers"
	"github.com/InWheelOrg/inwheel-api/internal/unmatched"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

func TestEnqueue_FirstInsert(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	repo := unmatched.NewRepository(db)

	clock := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	u := models.UnmatchedExternal{
		Source:        "wheelmap",
		SourceID:      "first",
		Lat:           46.4628,
		Lng:           6.8417,
		Attempts:      1,
		LastAttempted: clock,
		Payload:       json.RawMessage(`{"first":1}`),
	}

	if err := repo.Enqueue(ctx, u); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}

	var count int
	row := sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM unmatched_external WHERE source = $1 AND source_id = $2",
		"wheelmap", "first",
	)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	var gotSource, gotSourceID string
	var gotAttempts int
	var gotColLat, gotColLng float64
	var gotGeomLng, gotGeomLat float64
	var gotPayload []byte
	var gotLastAttempted time.Time

	row = sqlDB.QueryRowContext(ctx, `
		SELECT source, source_id, attempts, lat, lng,
		       ST_X(geom::geometry), ST_Y(geom::geometry),
		       payload, last_attempted
		FROM unmatched_external
		WHERE source = $1 AND source_id = $2
	`, "wheelmap", "first")
	if err := row.Scan(&gotSource, &gotSourceID, &gotAttempts, &gotColLat, &gotColLng, &gotGeomLng, &gotGeomLat, &gotPayload, &gotLastAttempted); err != nil {
		t.Fatalf("scan: %v", err)
	}

	if gotSource != "wheelmap" {
		t.Errorf("source = %q, want %q", gotSource, "wheelmap")
	}
	if gotSourceID != "first" {
		t.Errorf("source_id = %q, want %q", gotSourceID, "first")
	}
	if gotAttempts != 1 {
		t.Errorf("attempts = %d, want 1", gotAttempts)
	}
	if gotColLat != 46.4628 {
		t.Errorf("lat = %v, want %v", gotColLat, 46.4628)
	}
	if gotColLng != 6.8417 {
		t.Errorf("lng = %v, want %v", gotColLng, 6.8417)
	}
	if math.Abs(gotGeomLng-6.8417) >= 1e-9 {
		t.Errorf("geom X (lng) = %v, want %v", gotGeomLng, 6.8417)
	}
	if math.Abs(gotGeomLat-46.4628) >= 1e-9 {
		t.Errorf("geom Y (lat) = %v, want %v", gotGeomLat, 46.4628)
	}
	var gotPayloadMap, wantPayloadMap map[string]interface{}
	if err := json.Unmarshal(gotPayload, &gotPayloadMap); err != nil {
		t.Fatalf("unmarshal got payload: %v", err)
	}
	if err := json.Unmarshal([]byte(`{"first":1}`), &wantPayloadMap); err != nil {
		t.Fatalf("unmarshal want payload: %v", err)
	}
	if len(gotPayloadMap) != len(wantPayloadMap) || gotPayloadMap["first"] != wantPayloadMap["first"] {
		t.Errorf("payload = %q, want %q", string(gotPayload), `{"first":1}`)
	}
	if gotLastAttempted.UnixMicro() != clock.UnixMicro() {
		t.Errorf("last_attempted = %v, want %v", gotLastAttempted, clock)
	}
}

func TestEnqueue_ConflictBumpsAttempts(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	repo := unmatched.NewRepository(db)

	t1 := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	first := models.UnmatchedExternal{
		Source:        "wheelmap",
		SourceID:      "conflict",
		Lat:           46.4628,
		Lng:           6.8417,
		Attempts:      1,
		LastAttempted: t1,
		Payload:       json.RawMessage(`{"first":1}`),
	}
	if err := repo.Enqueue(ctx, first); err != nil {
		t.Fatalf("first Enqueue: %v", err)
	}

	t2 := t1.Add(time.Hour)
	second := models.UnmatchedExternal{
		Source:        "wheelmap",
		SourceID:      "conflict",
		Lat:           46.4628,
		Lng:           6.8417,
		Attempts:      1,
		LastAttempted: t2,
		Payload:       json.RawMessage(`{"second":2}`),
	}
	if err := repo.Enqueue(ctx, second); err != nil {
		t.Fatalf("second Enqueue: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}

	var count int
	row := sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM unmatched_external WHERE source = $1 AND source_id = $2",
		"wheelmap", "conflict",
	)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1 (upsert should not duplicate)", count)
	}

	var gotAttempts int
	var gotPayload []byte
	var gotLastAttempted time.Time

	row = sqlDB.QueryRowContext(ctx,
		"SELECT attempts, payload, last_attempted FROM unmatched_external WHERE source = $1 AND source_id = $2",
		"wheelmap", "conflict",
	)
	if err := row.Scan(&gotAttempts, &gotPayload, &gotLastAttempted); err != nil {
		t.Fatalf("scan: %v", err)
	}

	if gotAttempts != 2 {
		t.Errorf("attempts = %d, want 2", gotAttempts)
	}
	var gotPayloadMap map[string]interface{}
	if err := json.Unmarshal(gotPayload, &gotPayloadMap); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if v, ok := gotPayloadMap["second"]; !ok || v != float64(2) {
		t.Errorf("payload = %q, want {\"second\":2}", string(gotPayload))
	}
	if _, hasFirst := gotPayloadMap["first"]; hasFirst {
		t.Errorf("payload still contains first key: %q", string(gotPayload))
	}
	if gotLastAttempted.UnixMicro() != t2.UnixMicro() {
		t.Errorf("last_attempted = %v, want %v", gotLastAttempted, t2)
	}

	t3 := t1.Add(2 * time.Hour)
	third := models.UnmatchedExternal{
		Source:        "wheelmap",
		SourceID:      "conflict",
		Lat:           46.4628,
		Lng:           6.8417,
		Attempts:      1,
		LastAttempted: t3,
		Payload:       json.RawMessage(`{"third":3}`),
	}
	if err := repo.Enqueue(ctx, third); err != nil {
		t.Fatalf("third Enqueue: %v", err)
	}
	row = sqlDB.QueryRowContext(ctx,
		"SELECT attempts FROM unmatched_external WHERE source = $1 AND source_id = $2",
		"wheelmap", "conflict",
	)
	if err := row.Scan(&gotAttempts); err != nil {
		t.Fatalf("scan attempts: %v", err)
	}
	if gotAttempts != 3 {
		t.Errorf("after third enqueue attempts = %d, want 3 (must increment existing, not use caller's value)", gotAttempts)
	}
}

func TestEnqueue_ConflictRefreshesCoordinates(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	repo := unmatched.NewRepository(db)
	clock := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	first := models.UnmatchedExternal{
		Source: "wheelmap", SourceID: "moved",
		Lat: 46.4628, Lng: 6.8417, Attempts: 1, LastAttempted: clock,
		Payload: json.RawMessage(`{}`),
	}
	if err := repo.Enqueue(ctx, first); err != nil {
		t.Fatalf("first Enqueue: %v", err)
	}

	second := models.UnmatchedExternal{
		Source: "wheelmap", SourceID: "moved",
		Lat: 46.5000, Lng: 6.9000, Attempts: 1, LastAttempted: clock.Add(time.Hour),
		Payload: json.RawMessage(`{}`),
	}
	if err := repo.Enqueue(ctx, second); err != nil {
		t.Fatalf("second Enqueue: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	var gotLat, gotLng, gotGeomLng, gotGeomLat float64
	row := sqlDB.QueryRowContext(ctx, `
		SELECT lat, lng, ST_X(geom::geometry), ST_Y(geom::geometry)
		FROM unmatched_external
		WHERE source = $1 AND source_id = $2
	`, "wheelmap", "moved")
	if err := row.Scan(&gotLat, &gotLng, &gotGeomLng, &gotGeomLat); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if gotLat != 46.5000 || gotLng != 6.9000 {
		t.Errorf("lat/lng = %v/%v, want 46.5000/6.9000", gotLat, gotLng)
	}
	if math.Abs(gotGeomLng-6.9000) >= 1e-9 || math.Abs(gotGeomLat-46.5000) >= 1e-9 {
		t.Errorf("geom X/Y = %v/%v, want 6.9000/46.5000", gotGeomLng, gotGeomLat)
	}
}

func TestEnqueue_DistinctPairsCoexist(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	repo := unmatched.NewRepository(db)
	clock := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	alpha := models.UnmatchedExternal{
		Source: "wheelmap", SourceID: "alpha",
		Lat: 46.4628, Lng: 6.8417, Attempts: 1, LastAttempted: clock,
		Payload: json.RawMessage(`{"x":1}`),
	}
	beta := models.UnmatchedExternal{
		Source: "wheelmap", SourceID: "beta",
		Lat: 46.4630, Lng: 6.8420, Attempts: 1, LastAttempted: clock,
		Payload: json.RawMessage(`{"x":2}`),
	}
	if err := repo.Enqueue(ctx, alpha); err != nil {
		t.Fatalf("Enqueue alpha: %v", err)
	}
	if err := repo.Enqueue(ctx, beta); err != nil {
		t.Fatalf("Enqueue beta: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	var count int
	if err := sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM unmatched_external WHERE source = $1",
		"wheelmap",
	).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 2 {
		t.Errorf("row count = %d, want 2 (unique constraint must be on (source, source_id), not just source)", count)
	}
}
