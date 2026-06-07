/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/InWheelOrg/inwheel-api/internal/identity"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

type fakeSweepRepo struct {
	rows           []models.UnmatchedExternal
	findErr        error
	deleteCalls    []int64
	deleteErr      error
	bumpCalls      []bumpCall
	bumpErr        error
	lastTouchedIDs []string
	lastRadius     float64
}

type bumpCall struct {
	queueID       int64
	lastAttempted time.Time
}

func (f *fakeSweepRepo) FindCandidatesNearTouched(_ context.Context, ids []string, radiusM float64) ([]models.UnmatchedExternal, error) {
	f.lastTouchedIDs = ids
	f.lastRadius = radiusM
	return f.rows, f.findErr
}

func (f *fakeSweepRepo) Delete(_ context.Context, queueID int64) error {
	f.deleteCalls = append(f.deleteCalls, queueID)
	return f.deleteErr
}

func (f *fakeSweepRepo) BumpAttempts(_ context.Context, queueID int64, lastAttempted time.Time) error {
	f.bumpCalls = append(f.bumpCalls, bumpCall{queueID, lastAttempted})
	return f.bumpErr
}

func TestSweep_EmptyTouchedIDsDoesNothing(t *testing.T) {
	queue := &fakeSweepRepo{}
	cands := &candidatesRepo{}
	attach := &fakeAttachRepo{}
	s := &identity.Sweeper{Candidates: cands, Places: attach, Queue: queue, Now: time.Now}
	res, err := s.Sweep(context.Background(), nil)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res != (identity.SweepResult{}) {
		t.Errorf("result = %+v, want zero", res)
	}
	if queue.lastTouchedIDs != nil {
		t.Errorf("FindCandidatesNearTouched called: %v", queue.lastTouchedIDs)
	}
}

func TestSweep_PassesRadiusToRepo(t *testing.T) {
	queue := &fakeSweepRepo{rows: nil}
	s := &identity.Sweeper{
		Candidates: &candidatesRepo{},
		Places:     &fakeAttachRepo{},
		Queue:      queue,
		Now:        time.Now,
	}
	_, err := s.Sweep(context.Background(), []string{"p1"})
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if queue.lastRadius != identity.RadiusM {
		t.Errorf("radius = %v, want %v", queue.lastRadius, identity.RadiusM)
	}
}

func TestSweep_ConfidentAttachesAndDeletes(t *testing.T) {
	clock := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	queue := &fakeSweepRepo{rows: []models.UnmatchedExternal{
		{
			ID:     42,
			Source: "wheelmap", SourceID: "abc",
			Name: "Pascal", Lat: 46.4628, Lng: 6.8417,
			Category: "cafe",
			Street:   "Rue du Simplon", HouseNumber: "10",
			Payload: json.RawMessage(`{"x":1}`),
		},
	}}
	cands := &candidatesRepo{candidates: []models.Place{
		{ID: "p1", Name: "Pascal", Lat: 46.4628, Lng: 6.8417, Category: models.CategoryCafe,
			Tags: models.PlaceTags{"addr:street": "Rue du Simplon", "addr:housenumber": "10"}},
	}}
	attach := &fakeAttachRepo{}
	s := &identity.Sweeper{Candidates: cands, Places: attach, Queue: queue, Now: fixedClock(clock)}
	res, err := s.Sweep(context.Background(), []string{"p1"})
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Confident != 1 || res.LowConfidence != 0 || res.NoMatch != 0 || res.Errors != 0 {
		t.Errorf("result = %+v, want Confident=1", res)
	}
	if len(attach.calls) != 1 {
		t.Fatalf("AttachExternalRef calls = %d, want 1", len(attach.calls))
	}
	if attach.calls[0].placeID != "p1" || attach.calls[0].source != "wheelmap" {
		t.Errorf("attach call = %+v", attach.calls[0])
	}
	if !attach.calls[0].ref.MatchedAt.Equal(clock) {
		t.Errorf("MatchedAt = %v, want %v", attach.calls[0].ref.MatchedAt, clock)
	}
	if len(queue.deleteCalls) != 1 || queue.deleteCalls[0] != 42 {
		t.Errorf("Delete calls = %v, want [42]", queue.deleteCalls)
	}
	if len(queue.bumpCalls) != 0 {
		t.Errorf("BumpAttempts unexpectedly called: %v", queue.bumpCalls)
	}
}

func TestSweep_NoMatchBumpsAttempts(t *testing.T) {
	clock := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	queue := &fakeSweepRepo{rows: []models.UnmatchedExternal{
		{
			ID:     7,
			Source: "wheelmap", SourceID: "ghost",
			Name: "Nowhere", Lat: 46.4628, Lng: 6.8417,
			Category: "cafe",
		},
	}}
	cands := &candidatesRepo{candidates: nil}
	attach := &fakeAttachRepo{}
	s := &identity.Sweeper{Candidates: cands, Places: attach, Queue: queue, Now: fixedClock(clock)}
	res, err := s.Sweep(context.Background(), []string{"p1"})
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.NoMatch != 1 || res.Confident != 0 {
		t.Errorf("result = %+v, want NoMatch=1", res)
	}
	if len(attach.calls) != 0 {
		t.Errorf("AttachExternalRef unexpectedly called: %v", attach.calls)
	}
	if len(queue.deleteCalls) != 0 {
		t.Errorf("Delete unexpectedly called: %v", queue.deleteCalls)
	}
	if len(queue.bumpCalls) != 1 || queue.bumpCalls[0].queueID != 7 || !queue.bumpCalls[0].lastAttempted.Equal(clock) {
		t.Errorf("BumpAttempts = %v, want [{7, %v}]", queue.bumpCalls, clock)
	}
}

func TestSweep_FindErrorIsFatal(t *testing.T) {
	wantErr := errors.New("db down")
	queue := &fakeSweepRepo{findErr: wantErr}
	s := &identity.Sweeper{
		Candidates: &candidatesRepo{},
		Places:     &fakeAttachRepo{},
		Queue:      queue,
		Now:        time.Now,
	}
	_, err := s.Sweep(context.Background(), []string{"p1"})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want wrap of %v", err, wantErr)
	}
}

func TestSweep_MatchErrorIsNonFatal(t *testing.T) {
	wantErr := errors.New("transient")
	queue := &fakeSweepRepo{rows: []models.UnmatchedExternal{
		{ID: 1, Source: "wheelmap", SourceID: "a", Lat: 46.4628, Lng: 6.8417, Category: "cafe"},
		{ID: 2, Source: "wheelmap", SourceID: "b", Lat: 46.4628, Lng: 6.8417, Category: "cafe"},
	}}
	cands := &candidatesRepo{err: wantErr}
	s := &identity.Sweeper{Candidates: cands, Places: &fakeAttachRepo{}, Queue: queue, Now: time.Now}
	res, err := s.Sweep(context.Background(), []string{"p1"})
	if err != nil {
		t.Fatalf("Sweep returned fatal err = %v", err)
	}
	if res.Errors != 2 {
		t.Errorf("Errors = %d, want 2", res.Errors)
	}
	if res.Considered != 2 {
		t.Errorf("Considered = %d, want 2", res.Considered)
	}
}

func TestSweep_AttachFailureSkipsDelete(t *testing.T) {
	queue := &fakeSweepRepo{rows: []models.UnmatchedExternal{
		{ID: 1, Source: "wheelmap", SourceID: "a",
			Name: "Pascal", Lat: 46.4628, Lng: 6.8417, Category: "cafe",
			Street: "Rue du Simplon", HouseNumber: "10"},
	}}
	cands := &candidatesRepo{candidates: []models.Place{
		{ID: "p1", Name: "Pascal", Lat: 46.4628, Lng: 6.8417, Category: models.CategoryCafe,
			Tags: models.PlaceTags{"addr:street": "Rue du Simplon", "addr:housenumber": "10"}},
	}}
	attach := &fakeAttachRepo{err: errors.New("attach failed")}
	s := &identity.Sweeper{Candidates: cands, Places: attach, Queue: queue, Now: time.Now}
	res, err := s.Sweep(context.Background(), []string{"p1"})
	if err != nil {
		t.Fatalf("Sweep returned fatal err = %v", err)
	}
	if res.Errors != 1 || res.Confident != 0 {
		t.Errorf("result = %+v, want Errors=1 Confident=0", res)
	}
	if len(queue.deleteCalls) != 0 {
		t.Errorf("Delete called after attach failure: %v", queue.deleteCalls)
	}
}
