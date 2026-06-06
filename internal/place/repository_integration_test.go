//go:build integration

/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package place_test

import (
	"context"
	"strings"
	"testing"
	"time"

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

func TestFindCandidates_EmptyCategoriesReturnsNil(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	repo := place.NewRepository(db)
	got, err := repo.FindCandidates(ctx, 46.4628, 6.8417, 50, nil)
	if err != nil {
		t.Fatalf("FindCandidates: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestFindCandidates_RadiusStatusCategoryFilters(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	repo := place.NewRepository(db)

	// At this latitude, ~0.000225 deg lat ≈ 25 m; ~0.000898 deg lat ≈ 100 m.
	const cLat, cLng = 46.4628, 6.8417

	seed := []models.Place{
		// near + matching category + active → returned
		{OSMID: 1, OSMType: models.OSMNode, Name: "Near Cafe", Lat: cLat + 0.0001, Lng: cLng, Category: models.CategoryCafe, Rank: models.RankEstablishment, Source: "osm", Status: models.PlaceStatusActive},
		// far (~100 m) + matching category + active → excluded by radius
		{OSMID: 2, OSMType: models.OSMNode, Name: "Far Cafe", Lat: cLat + 0.000898, Lng: cLng, Category: models.CategoryCafe, Rank: models.RankEstablishment, Source: "osm", Status: models.PlaceStatusActive},
		// near + wrong category + active → excluded by category filter
		{OSMID: 3, OSMType: models.OSMNode, Name: "Near Pharmacy", Lat: cLat, Lng: cLng + 0.0001, Category: models.CategoryHealthcare, Rank: models.RankEstablishment, Source: "osm", Status: models.PlaceStatusActive},
		// near + matching category + closed → excluded by status filter
		{OSMID: 4, OSMType: models.OSMNode, Name: "Closed Cafe", Lat: cLat - 0.0001, Lng: cLng, Category: models.CategoryCafe, Rank: models.RankEstablishment, Source: "osm", Status: models.PlaceStatusClosed},
	}
	if err := repo.UpsertBatch(ctx, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := repo.FindCandidates(ctx, cLat, cLng, 50, []models.Category{models.CategoryCafe})
	if err != nil {
		t.Fatalf("FindCandidates: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1: %v", len(got), names(got))
	}
	if got[0].Name != "Near Cafe" {
		t.Errorf("got %q, want %q", got[0].Name, "Near Cafe")
	}
}

func TestFindCandidates_OrdersByDistance(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	repo := place.NewRepository(db)

	const cLat, cLng = 46.4628, 6.8417

	seed := []models.Place{
		// ~40 m north
		{OSMID: 1, OSMType: models.OSMNode, Name: "Far", Lat: cLat + 0.00036, Lng: cLng, Category: models.CategoryCafe, Rank: models.RankEstablishment, Source: "osm", Status: models.PlaceStatusActive},
		// ~10 m north (closer)
		{OSMID: 2, OSMType: models.OSMNode, Name: "Near", Lat: cLat + 0.00009, Lng: cLng, Category: models.CategoryCafe, Rank: models.RankEstablishment, Source: "osm", Status: models.PlaceStatusActive},
	}
	if err := repo.UpsertBatch(ctx, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := repo.FindCandidates(ctx, cLat, cLng, 50, []models.Category{models.CategoryCafe})
	if err != nil {
		t.Fatalf("FindCandidates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d candidates, want 2", len(got))
	}
	if got[0].Name != "Near" || got[1].Name != "Far" {
		t.Errorf("order = [%q, %q], want [\"Near\", \"Far\"]", got[0].Name, got[1].Name)
	}
}

func names(ps []models.Place) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
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

func TestAttachExternalRef_EmptyMap(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	repo := place.NewRepository(db)

	seed := []models.Place{
		{OSMID: 10, OSMType: models.OSMNode, Name: "Vevey Cafe", Lat: 46.4628, Lng: 6.8417, Category: models.CategoryCafe, Rank: models.RankEstablishment, Source: "osm", Status: models.PlaceStatusActive},
	}
	if err := repo.UpsertBatch(ctx, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var seeded models.Place
	if err := db.Where("osm_id = ?", 10).First(&seeded).Error; err != nil {
		t.Fatalf("fetch seeded place: %v", err)
	}

	matchedAt := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	ref := models.ExternalRef{ID: "wm/10", Confidence: 0.9, MatchedAt: matchedAt}
	if err := repo.AttachExternalRef(ctx, seeded.ID, "wheelmap", ref); err != nil {
		t.Fatalf("AttachExternalRef: %v", err)
	}

	var got models.Place
	if err := db.Where("osm_id = ?", 10).First(&got).Error; err != nil {
		t.Fatalf("reload place: %v", err)
	}
	wm, ok := got.ExternalIDs["wheelmap"]
	if !ok {
		t.Fatal("external_ids[wheelmap] missing")
	}
	if wm.ID != "wm/10" {
		t.Errorf("ID = %q, want %q", wm.ID, "wm/10")
	}
	if wm.Confidence != 0.9 {
		t.Errorf("Confidence = %v, want 0.9", wm.Confidence)
	}
	if wm.MatchedAt.UnixMicro() != matchedAt.Truncate(time.Microsecond).UnixMicro() {
		t.Errorf("MatchedAt = %v, want %v", wm.MatchedAt, matchedAt)
	}
}

func TestAttachExternalRef_ExistingDifferentKey(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	repo := place.NewRepository(db)

	seed := []models.Place{
		{OSMID: 11, OSMType: models.OSMNode, Name: "Vevey Pharmacy", Lat: 46.4628, Lng: 6.8417, Category: models.CategoryHealthcare, Rank: models.RankEstablishment, Source: "osm", Status: models.PlaceStatusActive, ExternalIDs: models.ExternalIDs{"osm": models.ExternalRef{ID: "node/123", Confidence: 1.0}}},
	}
	if err := repo.UpsertBatch(ctx, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var seeded models.Place
	if err := db.Where("osm_id = ?", 11).First(&seeded).Error; err != nil {
		t.Fatalf("fetch seeded place: %v", err)
	}

	matchedAt := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	ref := models.ExternalRef{ID: "wm/11", Confidence: 0.85, MatchedAt: matchedAt}
	if err := repo.AttachExternalRef(ctx, seeded.ID, "wheelmap", ref); err != nil {
		t.Fatalf("AttachExternalRef: %v", err)
	}

	var got models.Place
	if err := db.Where("osm_id = ?", 11).First(&got).Error; err != nil {
		t.Fatalf("reload place: %v", err)
	}
	if got.ExternalIDs["osm"].ID != "node/123" {
		t.Errorf("osm entry changed: ID = %q, want %q", got.ExternalIDs["osm"].ID, "node/123")
	}
	if got.ExternalIDs["osm"].Confidence != 1.0 {
		t.Errorf("osm Confidence = %v, want 1.0", got.ExternalIDs["osm"].Confidence)
	}
	wm, ok := got.ExternalIDs["wheelmap"]
	if !ok {
		t.Fatal("external_ids[wheelmap] missing")
	}
	if wm.ID != "wm/11" {
		t.Errorf("wheelmap ID = %q, want %q", wm.ID, "wm/11")
	}
	if wm.Confidence != 0.85 {
		t.Errorf("wheelmap Confidence = %v, want 0.85", wm.Confidence)
	}
}

func TestAttachExternalRef_ExistingSameKey(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	repo := place.NewRepository(db)

	oldTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	seed := []models.Place{
		{OSMID: 12, OSMType: models.OSMNode, Name: "Vevey Restaurant", Lat: 46.4628, Lng: 6.8417, Category: models.CategoryRestaurant, Rank: models.RankEstablishment, Source: "osm", Status: models.PlaceStatusActive, ExternalIDs: models.ExternalIDs{"wheelmap": models.ExternalRef{ID: "old", Confidence: 0.6, MatchedAt: oldTime}}},
	}
	if err := repo.UpsertBatch(ctx, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var seeded models.Place
	if err := db.Where("osm_id = ?", 12).First(&seeded).Error; err != nil {
		t.Fatalf("fetch seeded place: %v", err)
	}

	newTime := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	newRef := models.ExternalRef{ID: "new", Confidence: 0.95, MatchedAt: newTime}
	if err := repo.AttachExternalRef(ctx, seeded.ID, "wheelmap", newRef); err != nil {
		t.Fatalf("AttachExternalRef: %v", err)
	}

	var got models.Place
	if err := db.Where("osm_id = ?", 12).First(&got).Error; err != nil {
		t.Fatalf("reload place: %v", err)
	}
	wm, ok := got.ExternalIDs["wheelmap"]
	if !ok {
		t.Fatal("external_ids[wheelmap] missing after overwrite")
	}
	if wm.ID != "new" {
		t.Errorf("ID = %q, want %q", wm.ID, "new")
	}
	if wm.Confidence != 0.95 {
		t.Errorf("Confidence = %v, want 0.95", wm.Confidence)
	}
	if wm.MatchedAt.UnixMicro() != newTime.Truncate(time.Microsecond).UnixMicro() {
		t.Errorf("MatchedAt = %v, want %v", wm.MatchedAt, newTime)
	}
}

func TestAttachExternalRef_PlaceNotFound(t *testing.T) {
	ctx := context.Background()
	db, cleanup, err := testhelpers.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	repo := place.NewRepository(db)

	ref := models.ExternalRef{ID: "wm/999", Confidence: 0.9, MatchedAt: time.Now()}
	err = repo.AttachExternalRef(ctx, "00000000-0000-0000-0000-000000000000", "wheelmap", ref)
	if err == nil {
		t.Fatal("expected error for non-existent place, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "not found")
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
