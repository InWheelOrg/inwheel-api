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
		Name:          "Test Cafe",
		Category:      "cafe",
		Street:        "Rue du Simplon",
		HouseNumber:   "10",
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

	var gotName, gotCategory, gotStreet, gotHouseNumber string
	row2 := sqlDB.QueryRowContext(ctx,
		"SELECT name, category, street, housenumber FROM unmatched_external WHERE source = $1 AND source_id = $2",
		"wheelmap", "first",
	)
	if err := row2.Scan(&gotName, &gotCategory, &gotStreet, &gotHouseNumber); err != nil {
		t.Fatalf("scan matchable fields: %v", err)
	}
	if gotName != "Test Cafe" {
		t.Errorf("name = %q, want %q", gotName, "Test Cafe")
	}
	if gotCategory != "cafe" {
		t.Errorf("category = %q, want %q", gotCategory, "cafe")
	}
	if gotStreet != "Rue du Simplon" {
		t.Errorf("street = %q, want %q", gotStreet, "Rue du Simplon")
	}
	if gotHouseNumber != "10" {
		t.Errorf("housenumber = %q, want %q", gotHouseNumber, "10")
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
		Name:          "Original Name",
		Category:      "cafe",
		Street:        "Old Street",
		HouseNumber:   "1",
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
		Name:          "Updated Name",
		Category:      "restaurant",
		Street:        "New Street",
		HouseNumber:   "2",
		Lat:           46.4630,
		Lng:           6.8419,
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

	// Verify matchable fields were overwritten by second enqueue
	var gotName, gotCategory, gotStreet, gotHouseNumber string
	if err := sqlDB.QueryRowContext(ctx,
		"SELECT name, category, street, housenumber FROM unmatched_external WHERE source = $1 AND source_id = $2",
		"wheelmap", "conflict",
	).Scan(&gotName, &gotCategory, &gotStreet, &gotHouseNumber); err != nil {
		t.Fatalf("scan matchable fields: %v", err)
	}
	if gotName != "Updated Name" {
		t.Errorf("name = %q, want %q", gotName, "Updated Name")
	}
	if gotCategory != "restaurant" {
		t.Errorf("category = %q, want %q", gotCategory, "restaurant")
	}
	if gotStreet != "New Street" {
		t.Errorf("street = %q, want %q", gotStreet, "New Street")
	}
	if gotHouseNumber != "2" {
		t.Errorf("housenumber = %q, want %q", gotHouseNumber, "2")
	}

	// Third enqueue to verify attempts keeps incrementing
	t3 := t1.Add(2 * time.Hour)
	third := models.UnmatchedExternal{
		Source:        "wheelmap",
		SourceID:      "conflict",
		Name:          "Third Name",
		Category:      "bar",
		Street:        "Third Street",
		HouseNumber:   "3",
		Lat:           46.4635,
		Lng:           6.8425,
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
	// Verify matchable fields overwritten by third
	if err := sqlDB.QueryRowContext(ctx,
		"SELECT name, category, street, housenumber FROM unmatched_external WHERE source = $1 AND source_id = $2",
		"wheelmap", "conflict",
	).Scan(&gotName, &gotCategory, &gotStreet, &gotHouseNumber); err != nil {
		t.Fatalf("scan matchable fields third: %v", err)
	}
	if gotName != "Third Name" {
		t.Errorf("name after third = %q, want %q", gotName, "Third Name")
	}
	if gotCategory != "bar" {
		t.Errorf("category after third = %q, want %q", gotCategory, "bar")
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
		Name: "Original Name", Category: "cafe", Street: "Old St", HouseNumber: "1",
		Lat: 46.4628, Lng: 6.8417, Attempts: 1, LastAttempted: clock,
		Payload: json.RawMessage(`{}`),
	}
	if err := repo.Enqueue(ctx, first); err != nil {
		t.Fatalf("first Enqueue: %v", err)
	}

	second := models.UnmatchedExternal{
		Source: "wheelmap", SourceID: "moved",
		Name: "Updated Name", Category: "restaurant", Street: "Updated St", HouseNumber: "2",
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
	var gotLat, gotLng float64
	var gotGeomLng, gotGeomLat float64
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

	var gotName, gotCategory, gotStreet, gotHouseNumber string
	if err := sqlDB.QueryRowContext(ctx,
		"SELECT name, category, street, housenumber FROM unmatched_external WHERE source = $1 AND source_id = $2",
		"wheelmap", "moved",
	).Scan(&gotName, &gotCategory, &gotStreet, &gotHouseNumber); err != nil {
		t.Fatalf("scan matchable: %v", err)
	}
	if gotName != "Updated Name" {
		t.Errorf("name = %q, want %q", gotName, "Updated Name")
	}
	if gotCategory != "restaurant" {
		t.Errorf("category = %q, want %q", gotCategory, "restaurant")
	}
	if gotStreet != "Updated St" {
		t.Errorf("street = %q, want %q", gotStreet, "Updated St")
	}
	if gotHouseNumber != "2" {
		t.Errorf("housenumber = %q, want %q", gotHouseNumber, "2")
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
		Name: "Alpha Place", Category: "cafe", Street: "Alpha St", HouseNumber: "1",
		Lat: 46.4628, Lng: 6.8417, Attempts: 1, LastAttempted: clock,
		Payload: json.RawMessage(`{"x":1}`),
	}
	beta := models.UnmatchedExternal{
		Source: "wheelmap", SourceID: "beta",
		Name: "Beta Place", Category: "shop", Street: "Beta Ave", HouseNumber: "2",
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

	var alphaName, betaName string
	if err := sqlDB.QueryRowContext(ctx,
		"SELECT name FROM unmatched_external WHERE source = $1 AND source_id = $2",
		"wheelmap", "alpha",
	).Scan(&alphaName); err != nil {
		t.Fatalf("scan alpha: %v", err)
	}
	if err := sqlDB.QueryRowContext(ctx,
		"SELECT name FROM unmatched_external WHERE source = $1 AND source_id = $2",
		"wheelmap", "beta",
	).Scan(&betaName); err != nil {
		t.Fatalf("scan beta: %v", err)
	}
	if alphaName != "Alpha Place" {
		t.Errorf("alpha name = %q, want %q", alphaName, "Alpha Place")
	}
	if betaName != "Beta Place" {
		t.Errorf("beta name = %q, want %q", betaName, "Beta Place")
	}
}
