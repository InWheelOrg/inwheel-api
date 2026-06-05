/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package identity_test

import (
	"context"
	"errors"
	"testing"

	"github.com/InWheelOrg/inwheel-api/internal/identity"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// fakeRepo returns a fixed candidate slice and records the categories it was
// called with so tests can assert the compat-filter was applied.
type fakeRepo struct {
	candidates []models.Place
	err        error
	lastCats   []models.Category
}

func (f *fakeRepo) FindCandidates(_ context.Context, _, _, _ float64, cats []models.Category) ([]models.Place, error) {
	f.lastCats = cats
	return f.candidates, f.err
}

func TestMatch_NoCandidatesReturnsNoMatch(t *testing.T) {
	repo := &fakeRepo{candidates: nil}
	r := identity.Record{
		Name:     "Pascal",
		Lat:      60.1699,
		Lng:      24.9384,
		Category: models.CategoryCafe,
	}
	d, err := identity.Match(context.Background(), repo, r)
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if d.Kind != identity.KindNoMatch {
		t.Errorf("Kind = %v, want KindNoMatch", d.Kind)
	}
	if d.MatchedPlaceID != "" {
		t.Errorf("MatchedPlaceID = %q, want empty", d.MatchedPlaceID)
	}
	if d.Confidence != 0 {
		t.Errorf("Confidence = %v, want 0", d.Confidence)
	}
}

func TestMatch_PassesCompatibleCategoriesToRepo(t *testing.T) {
	repo := &fakeRepo{candidates: nil}
	r := identity.Record{
		Name:     "Pascal",
		Lat:      60.1699,
		Lng:      24.9384,
		Category: models.CategoryCafe,
	}
	if _, err := identity.Match(context.Background(), repo, r); err != nil {
		t.Fatalf("Match: %v", err)
	}
	want := []models.Category{models.CategoryBar, models.CategoryCafe, models.CategoryRestaurant}
	if len(repo.lastCats) != len(want) {
		t.Fatalf("lastCats = %v, want %v", repo.lastCats, want)
	}
	for i := range want {
		if repo.lastCats[i] != want[i] {
			t.Errorf("lastCats[%d] = %q, want %q", i, repo.lastCats[i], want[i])
		}
	}
}

func TestMatch_PropagatesRepoError(t *testing.T) {
	wantErr := errors.New("db down")
	repo := &fakeRepo{err: wantErr}
	r := identity.Record{Lat: 60, Lng: 24, Category: models.CategoryCafe}
	_, err := identity.Match(context.Background(), repo, r)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestMatch_ConfidentAttachOnHighScore(t *testing.T) {
	// Coincident point, identical name, matching address → score = 1.0
	repo := &fakeRepo{candidates: []models.Place{
		{ID: "p1", Name: "Pascal", Lat: 60.1699, Lng: 24.9384, Category: models.CategoryCafe,
			Tags: models.PlaceTags{"addr:street": "Mannerheimintie", "addr:housenumber": "10"}},
	}}
	r := identity.Record{
		Name: "Pascal", Lat: 60.1699, Lng: 24.9384, Category: models.CategoryCafe,
		Street: "Mannerheimintie", HouseNumber: "10",
	}
	d, err := identity.Match(context.Background(), repo, r)
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if d.Kind != identity.KindConfident {
		t.Errorf("Kind = %v, want KindConfident", d.Kind)
	}
	if d.MatchedPlaceID != "p1" {
		t.Errorf("MatchedPlaceID = %q, want %q", d.MatchedPlaceID, "p1")
	}
	if d.Confidence < 0.99 {
		t.Errorf("Confidence = %v, want >= 0.99", d.Confidence)
	}
}

func TestMatch_LowConfidenceAttachInMiddleBand(t *testing.T) {
	// ~25 m offset (distance score ~0.5), name fully overlaps (1.0), no address.
	// Score with redistribution: 0.5556*0.5 + 0.4444*1.0 = 0.7222 → low confidence.
	repo := &fakeRepo{candidates: []models.Place{
		{ID: "p1", Name: "Pascal", Lat: 60.169675, Lng: 24.9384, Category: models.CategoryCafe},
	}}
	r := identity.Record{Name: "Pascal", Lat: 60.1699, Lng: 24.9384, Category: models.CategoryCafe}
	d, err := identity.Match(context.Background(), repo, r)
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if d.Kind != identity.KindLowConfidence {
		t.Errorf("Kind = %v (confidence %v), want KindLowConfidence", d.Kind, d.Confidence)
	}
	if d.MatchedPlaceID != "p1" {
		t.Errorf("MatchedPlaceID = %q, want %q", d.MatchedPlaceID, "p1")
	}
	if d.Confidence < 0.55 || d.Confidence >= 0.80 {
		t.Errorf("Confidence = %v, want in [0.55, 0.80)", d.Confidence)
	}
}

func TestMatch_BelowFloorReturnsNoMatch(t *testing.T) {
	// ~40 m offset (distance ~0.2), name no overlap, no address.
	// Score with redistribution: 0.5556*0.2 + 0.4444*0 = 0.111 → below floor.
	repo := &fakeRepo{candidates: []models.Place{
		{ID: "p1", Name: "Roma", Lat: 60.17026, Lng: 24.9384, Category: models.CategoryCafe},
	}}
	r := identity.Record{Name: "Pascal", Lat: 60.1699, Lng: 24.9384, Category: models.CategoryCafe}
	d, err := identity.Match(context.Background(), repo, r)
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if d.Kind != identity.KindNoMatch {
		t.Errorf("Kind = %v (confidence %v), want KindNoMatch", d.Kind, d.Confidence)
	}
	if d.MatchedPlaceID != "" {
		t.Errorf("MatchedPlaceID = %q, want empty", d.MatchedPlaceID)
	}
}

func TestMatch_PicksHighestScoringCandidate(t *testing.T) {
	// Two candidates at the same point. One has a matching name, the other doesn't.
	repo := &fakeRepo{candidates: []models.Place{
		{ID: "p1", Name: "Roma", Lat: 60.1699, Lng: 24.9384, Category: models.CategoryCafe},
		{ID: "p2", Name: "Pascal", Lat: 60.1699, Lng: 24.9384, Category: models.CategoryCafe},
	}}
	r := identity.Record{Name: "Pascal", Lat: 60.1699, Lng: 24.9384, Category: models.CategoryCafe}
	d, err := identity.Match(context.Background(), repo, r)
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if d.MatchedPlaceID != "p2" {
		t.Errorf("MatchedPlaceID = %q, want %q (highest-scoring)", d.MatchedPlaceID, "p2")
	}
}
