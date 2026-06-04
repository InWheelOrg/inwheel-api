//go:build integration

/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package place_test

import (
	"context"
	"testing"

	"github.com/InWheelOrg/inwheel-api/internal/place"
	"github.com/InWheelOrg/inwheel-api/internal/testhelpers"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

func TestUpsertBatch_InsertsNewPlaces(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	repo := place.NewRepository(db)

	places := []models.Place{
		{OSMID: 1, OSMType: models.OSMNode, Name: "A", Lat: 60, Lng: 24, Category: models.CategoryRestaurant, Rank: models.RankEstablishment, Source: "osm", Status: models.PlaceStatusActive, ExternalIDs: models.ExternalIDs{"osm": models.ExternalRef{ID: "node/1", Confidence: 1.0}}},
		{OSMID: 2, OSMType: models.OSMNode, Name: "B", Lat: 61, Lng: 25, Category: models.CategoryCafe, Rank: models.RankEstablishment, Source: "osm", Status: models.PlaceStatusActive, ExternalIDs: models.ExternalIDs{"osm": models.ExternalRef{ID: "node/2", Confidence: 1.0}}},
	}

	if err := repo.UpsertBatch(ctx, places); err != nil {
		t.Fatalf("upsert batch: %v", err)
	}

	var count int64
	db.Model(&models.Place{}).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 rows, got %d", count)
	}
	var inserted models.Place
	if err := db.Where("osm_id = ?", 1).First(&inserted).Error; err != nil {
		t.Fatalf("fetch inserted place: %v", err)
	}
	if inserted.ExternalIDs["osm"].ID != "node/1" {
		t.Errorf("external_ids[osm].id = %q, want %q", inserted.ExternalIDs["osm"].ID, "node/1")
	}
	if inserted.ExternalIDs["osm"].Confidence != 1.0 {
		t.Errorf("external_ids[osm].confidence = %v, want 1.0", inserted.ExternalIDs["osm"].Confidence)
	}
}

func TestUpsertBatch_UpdatesExistingPlace(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	repo := place.NewRepository(db)

	first := []models.Place{{OSMID: 42, OSMType: models.OSMNode, Name: "Original", Lat: 60, Lng: 24, Category: models.CategoryRestaurant, Rank: models.RankEstablishment, Source: "osm", Status: models.PlaceStatusActive, ExternalIDs: models.ExternalIDs{"osm": models.ExternalRef{ID: "node/42", Confidence: 1.0}}}}
	if err := repo.UpsertBatch(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	second := []models.Place{{OSMID: 42, OSMType: models.OSMNode, Name: "Renamed", Lat: 60.5, Lng: 24.5, Category: models.CategoryRestaurant, Rank: models.RankEstablishment, Source: "osm", Status: models.PlaceStatusActive, ExternalIDs: models.ExternalIDs{"osm": models.ExternalRef{ID: "node/42", Confidence: 1.0}}}}
	if err := repo.UpsertBatch(ctx, second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	var got models.Place
	if err := db.Where("osm_id = ?", 42).First(&got).Error; err != nil {
		t.Fatalf("fetch updated row: %v", err)
	}
	if got.Name != "Renamed" {
		t.Errorf("name was not updated: got %q", got.Name)
	}
	if got.Lat != 60.5 || got.Lng != 24.5 {
		t.Errorf("coords were not updated: got (%v, %v)", got.Lat, got.Lng)
	}
	if got.ExternalIDs["osm"].ID != "node/42" {
		t.Errorf("external_ids[osm].id = %q, want %q", got.ExternalIDs["osm"].ID, "node/42")
	}
	if got.ExternalIDs["osm"].Confidence != 1.0 {
		t.Errorf("external_ids[osm].confidence = %v, want 1.0", got.ExternalIDs["osm"].Confidence)
	}
}

func TestUpsertBatch_EmptySliceIsNoOp(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	repo := place.NewRepository(db)
	if err := repo.UpsertBatch(ctx, nil); err != nil {
		t.Fatalf("nil batch should be a no-op, got error: %v", err)
	}
	if err := repo.UpsertBatch(ctx, []models.Place{}); err != nil {
		t.Fatalf("empty batch should be a no-op, got error: %v", err)
	}
}

func TestUnmatchedExternal_TableRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}

	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO unmatched_external (source, source_id, payload, lat, lng, geom)
		VALUES ($1, $2, $3, $4, $5, ST_SetSRID(ST_MakePoint($5, $4), 4326))
	`, "wheelmap", "wm/456", `{"name":"Test Cafe","category":"cafe"}`, 60.1699, 24.9384)
	if err != nil {
		t.Fatalf("insert into unmatched_external: %v", err)
	}

	var count int
	row := sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM unmatched_external WHERE source = $1 AND source_id = $2",
		"wheelmap", "wm/456",
	)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// Verify geometry is stored with correct coordinate order (lng=X, lat=Y).
	var storedLng, storedLat float64
	geomRow := sqlDB.QueryRowContext(ctx,
		"SELECT ST_X(geom::geometry), ST_Y(geom::geometry) FROM unmatched_external WHERE source = $1 AND source_id = $2",
		"wheelmap", "wm/456",
	)
	if err := geomRow.Scan(&storedLng, &storedLat); err != nil {
		t.Fatalf("geom query: %v", err)
	}
	if storedLng < 24.938 || storedLng > 24.939 {
		t.Errorf("geom X (lng) = %v, want ~24.9384", storedLng)
	}
	if storedLat < 60.169 || storedLat > 60.171 {
		t.Errorf("geom Y (lat) = %v, want ~60.1699", storedLat)
	}
}

func TestUnmatchedExternal_UniqueConstraint(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	insert := func() error {
		_, err := sqlDB.ExecContext(ctx, `
			INSERT INTO unmatched_external (source, source_id, payload, lat, lng, geom)
			VALUES ($1, $2, $3, $4, $5, ST_SetSRID(ST_MakePoint($5, $4), 4326))
		`, "wheelmap", "wm/999", `{}`, 60.0, 25.0)
		return err
	}

	if err := insert(); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := insert(); err == nil {
		t.Error("second insert with same (source, source_id) should fail the UNIQUE constraint, got nil error")
	}
}

func TestUnmatchedExternal_ColumnDefaults(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}

	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO unmatched_external (source, source_id, payload, lat, lng, geom)
		VALUES ($1, $2, $3, $4, $5, ST_SetSRID(ST_MakePoint($5, $4), 4326))
	`, "wheelmap", "wm/defaults", `{}`, 60.0, 25.0)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	var attempts int
	var lastAttempted *string // non-null check via nullable scan
	row := sqlDB.QueryRowContext(ctx,
		"SELECT attempts, last_attempted::text FROM unmatched_external WHERE source = $1 AND source_id = $2",
		"wheelmap", "wm/defaults",
	)
	if err := row.Scan(&attempts, &lastAttempted); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
	if lastAttempted == nil {
		t.Error("last_attempted is NULL, want a timestamp from DEFAULT NOW()")
	}
}

func TestUnmatchedExternal_SpatialQuery(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}

	// Vevey, Switzerland — two records with a clear distance separation.
	// near: 46.4628, 6.8417 (Grand-Place)
	// far:  46.4608, 6.8417 (~222 m south — well outside a 50 m radius)
	insert := func(sourceID string, lat, lng float64) {
		_, err := sqlDB.ExecContext(ctx, `
			INSERT INTO unmatched_external (source, source_id, payload, lat, lng, geom)
			VALUES ($1, $2, $3, $4, $5, ST_SetSRID(ST_MakePoint($5, $4), 4326))
		`, "wheelmap", sourceID, `{}`, lat, lng)
		if err != nil {
			t.Fatalf("insert %s: %v", sourceID, err)
		}
	}
	insert("near", 46.4628, 6.8417)
	insert("far", 46.4608, 6.8417)

	// Query within 50 m of the near point — should return only "near".
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT source_id FROM unmatched_external
		WHERE ST_DWithin(geom, ST_SetSRID(ST_MakePoint(6.8417, 46.4628), 4326)::geography, 50)
	`)
	if err != nil {
		t.Fatalf("ST_DWithin query: %v", err)
	}
	defer rows.Close()

	var found []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		found = append(found, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	if len(found) != 1 || found[0] != "near" {
		t.Errorf("ST_DWithin(50m) returned %v, want [near]", found)
	}
}
