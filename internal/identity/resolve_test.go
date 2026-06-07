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

type fakeAttachRepo struct {
	calls []attachCall
	err   error
}

type attachCall struct {
	placeID string
	source  string
	ref     models.ExternalRef
}

func (f *fakeAttachRepo) AttachExternalRef(_ context.Context, placeID, source string, ref models.ExternalRef) error {
	f.calls = append(f.calls, attachCall{placeID, source, ref})
	return f.err
}

type fakeEnqueueRepo struct {
	calls []models.UnmatchedExternal
	err   error
}

func (f *fakeEnqueueRepo) Enqueue(_ context.Context, u models.UnmatchedExternal) error {
	f.calls = append(f.calls, u)
	return f.err
}

type candidatesRepo struct {
	candidates []models.Place
	err        error
}

func (c *candidatesRepo) FindCandidates(_ context.Context, _, _, _ float64, _ []models.Category) ([]models.Place, error) {
	return c.candidates, c.err
}

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestResolve_ConfidentAttachesExternalRef(t *testing.T) {
	clock := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	attach := &fakeAttachRepo{}
	enqueue := &fakeEnqueueRepo{}
	cands := &candidatesRepo{candidates: []models.Place{
		{ID: "p1", Name: "Pascal", Lat: 46.4628, Lng: 6.8417, Category: models.CategoryCafe,
			Tags: models.PlaceTags{"addr:street": "Rue du Simplon", "addr:housenumber": "10"}},
	}}
	r := &identity.Resolver{
		Candidates: cands, Places: attach, Unmatched: enqueue, Now: fixedClock(clock),
	}
	rec := identity.Record{
		Source: "wheelmap", SourceID: "abc",
		Name: "Pascal", Lat: 46.4628, Lng: 6.8417, Category: models.CategoryCafe,
		Street: "Rue du Simplon", HouseNumber: "10",
	}
	d, err := r.Resolve(context.Background(), rec)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if d.Kind != identity.KindConfident {
		t.Fatalf("Decision.Kind = %v, want KindConfident", d.Kind)
	}
	if len(attach.calls) != 1 {
		t.Fatalf("AttachExternalRef calls = %d, want 1", len(attach.calls))
	}
	got := attach.calls[0]
	if got.placeID != "p1" || got.source != "wheelmap" {
		t.Errorf("attach call = %+v, want placeID=p1 source=wheelmap", got)
	}
	if got.ref.ID != "abc" {
		t.Errorf("ref.ID = %q, want %q", got.ref.ID, "abc")
	}
	if got.ref.Confidence < 0.99 {
		t.Errorf("ref.Confidence = %v, want >= 0.99", got.ref.Confidence)
	}
	if !got.ref.MatchedAt.Equal(clock) {
		t.Errorf("ref.MatchedAt = %v, want %v", got.ref.MatchedAt, clock)
	}
	if len(enqueue.calls) != 0 {
		t.Errorf("Enqueue calls = %d, want 0", len(enqueue.calls))
	}
}

func TestResolve_LowConfidenceAttachesWithLowScore(t *testing.T) {
	clock := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	attach := &fakeAttachRepo{}
	enqueue := &fakeEnqueueRepo{}
	cands := &candidatesRepo{candidates: []models.Place{
		{ID: "p2", Name: "Pascal", Lat: 46.462575, Lng: 6.8417, Category: models.CategoryCafe},
	}}
	r := &identity.Resolver{
		Candidates: cands, Places: attach, Unmatched: enqueue, Now: fixedClock(clock),
	}
	rec := identity.Record{
		Source: "wheelmap", SourceID: "xyz",
		Name: "Pascal", Lat: 46.4628, Lng: 6.8417, Category: models.CategoryCafe,
	}
	d, err := r.Resolve(context.Background(), rec)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if d.Kind != identity.KindLowConfidence {
		t.Fatalf("Decision.Kind = %v (confidence %v), want KindLowConfidence", d.Kind, d.Confidence)
	}
	if len(attach.calls) != 1 {
		t.Fatalf("AttachExternalRef calls = %d, want 1", len(attach.calls))
	}
	got := attach.calls[0]
	if got.placeID != "p2" {
		t.Errorf("attach placeID = %q, want p2", got.placeID)
	}
	if got.ref.Confidence < 0.55 || got.ref.Confidence >= 0.80 {
		t.Errorf("ref.Confidence = %v, want in [0.55, 0.80)", got.ref.Confidence)
	}
	if !got.ref.MatchedAt.Equal(clock) {
		t.Errorf("ref.MatchedAt = %v, want %v", got.ref.MatchedAt, clock)
	}
	if len(enqueue.calls) != 0 {
		t.Errorf("Enqueue calls = %d, want 0", len(enqueue.calls))
	}
}

func TestResolve_NoMatchEnqueues(t *testing.T) {
	clock := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	attach := &fakeAttachRepo{}
	enqueue := &fakeEnqueueRepo{}
	cands := &candidatesRepo{candidates: nil}
	r := &identity.Resolver{
		Candidates: cands, Places: attach, Unmatched: enqueue, Now: fixedClock(clock),
	}
	payload := json.RawMessage(`{"raw":"data"}`)
	rec := identity.Record{
		Source: "wheelmap", SourceID: "ghost",
		Name: "Nowhere", Lat: 46.4628, Lng: 6.8417, Category: models.CategoryCafe,
		Street: "Rue du Simplon", HouseNumber: "10",
		Payload: payload,
	}
	d, err := r.Resolve(context.Background(), rec)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if d.Kind != identity.KindNoMatch {
		t.Fatalf("Decision.Kind = %v, want KindNoMatch", d.Kind)
	}
	if len(attach.calls) != 0 {
		t.Errorf("AttachExternalRef calls = %d, want 0", len(attach.calls))
	}
	if len(enqueue.calls) != 1 {
		t.Fatalf("Enqueue calls = %d, want 1", len(enqueue.calls))
	}
	got := enqueue.calls[0]
	if got.Source != "wheelmap" || got.SourceID != "ghost" {
		t.Errorf("enqueued source/id = %q/%q, want wheelmap/ghost", got.Source, got.SourceID)
	}
	if got.Lat != 46.4628 || got.Lng != 6.8417 {
		t.Errorf("enqueued lat/lng = %v/%v, want 46.4628/6.8417", got.Lat, got.Lng)
	}
	if got.Name != "Nowhere" {
		t.Errorf("enqueued Name = %q, want %q", got.Name, "Nowhere")
	}
	if got.Category != string(models.CategoryCafe) {
		t.Errorf("enqueued Category = %q, want %q", got.Category, models.CategoryCafe)
	}
	if got.Street != "Rue du Simplon" {
		t.Errorf("enqueued Street = %q, want %q", got.Street, "Rue du Simplon")
	}
	if got.HouseNumber != "10" {
		t.Errorf("enqueued HouseNumber = %q, want %q", got.HouseNumber, "10")
	}
	if string(got.Payload) != string(payload) {
		t.Errorf("enqueued payload = %s, want %s", got.Payload, payload)
	}
	if !got.LastAttempted.Equal(clock) {
		t.Errorf("enqueued LastAttempted = %v, want %v", got.LastAttempted, clock)
	}
	if got.Attempts != 1 {
		t.Errorf("enqueued Attempts = %d, want 1", got.Attempts)
	}
}

func TestResolve_NoMatchNilPayloadDefaultsToEmptyObject(t *testing.T) {
	enqueue := &fakeEnqueueRepo{}
	r := &identity.Resolver{
		Candidates: &candidatesRepo{candidates: nil},
		Places:     &fakeAttachRepo{},
		Unmatched:  enqueue,
		Now:        fixedClock(time.Now()),
	}
	_, err := r.Resolve(context.Background(), identity.Record{
		Source: "wheelmap", SourceID: "no-payload",
		Name: "Nowhere", Lat: 46.4628, Lng: 6.8417, Category: models.CategoryCafe,
		Payload: nil,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(enqueue.calls) != 1 {
		t.Fatalf("Enqueue calls = %d, want 1", len(enqueue.calls))
	}
	if string(enqueue.calls[0].Payload) != "{}" {
		t.Errorf("enqueued Payload = %s, want {}", enqueue.calls[0].Payload)
	}
}

func TestResolve_NilClockDefaultsToTimeNow(t *testing.T) {
	attach := &fakeAttachRepo{}
	enqueue := &fakeEnqueueRepo{}
	cands := &candidatesRepo{candidates: []models.Place{
		{ID: "p1", Name: "Pascal", Lat: 46.4628, Lng: 6.8417, Category: models.CategoryCafe},
	}}
	r := &identity.Resolver{
		Candidates: cands, Places: attach, Unmatched: enqueue, Now: nil,
	}
	before := time.Now()
	_, err := r.Resolve(context.Background(), identity.Record{
		Source: "wheelmap", SourceID: "x",
		Name: "Pascal", Lat: 46.4628, Lng: 6.8417, Category: models.CategoryCafe,
	})
	after := time.Now()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(attach.calls) != 1 {
		t.Fatalf("AttachExternalRef calls = %d, want 1", len(attach.calls))
	}
	got := attach.calls[0].ref.MatchedAt
	if got.Before(before) || got.After(after) {
		t.Errorf("MatchedAt = %v, want in [%v, %v]", got, before, after)
	}
}

func TestResolve_PropagatesMatchError(t *testing.T) {
	wantErr := errors.New("db down")
	r := &identity.Resolver{
		Candidates: &candidatesRepo{err: wantErr},
		Places:     &fakeAttachRepo{},
		Unmatched:  &fakeEnqueueRepo{},
		Now:        time.Now,
	}
	d, err := r.Resolve(context.Background(), identity.Record{
		Name: "x", Lat: 46.4628, Lng: 6.8417, Category: models.CategoryCafe,
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want wrap of %v", err, wantErr)
	}
	if d != (identity.Decision{}) {
		t.Errorf("Decision = %+v, want zero value", d)
	}
}

func TestResolve_PropagatesAttachError(t *testing.T) {
	wantErr := errors.New("attach failed")
	cands := &candidatesRepo{candidates: []models.Place{
		{ID: "p1", Name: "Pascal", Lat: 46.4628, Lng: 6.8417, Category: models.CategoryCafe},
	}}
	r := &identity.Resolver{
		Candidates: cands,
		Places:     &fakeAttachRepo{err: wantErr},
		Unmatched:  &fakeEnqueueRepo{},
		Now:        time.Now,
	}
	_, err := r.Resolve(context.Background(), identity.Record{
		Name: "Pascal", Lat: 46.4628, Lng: 6.8417, Category: models.CategoryCafe,
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want wrap of %v", err, wantErr)
	}
}

func TestResolve_PropagatesEnqueueError(t *testing.T) {
	wantErr := errors.New("enqueue failed")
	r := &identity.Resolver{
		Candidates: &candidatesRepo{candidates: nil},
		Places:     &fakeAttachRepo{},
		Unmatched:  &fakeEnqueueRepo{err: wantErr},
		Now:        time.Now,
	}
	_, err := r.Resolve(context.Background(), identity.Record{
		Name: "x", Lat: 46.4628, Lng: 6.8417, Category: models.CategoryCafe,
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want wrap of %v", err, wantErr)
	}
}
