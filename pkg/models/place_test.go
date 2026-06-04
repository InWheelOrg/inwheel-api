/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

func TestExternalIDs_ScanValue_RoundTrip(t *testing.T) {
	want := models.ExternalIDs{
		"osm": models.ExternalRef{ID: "node/123", Confidence: 1.0},
		"wheelmap": models.ExternalRef{
			ID:         "456",
			Confidence: 0.78,
			MatchedAt:  time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC),
		},
	}

	val, err := want.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	b, ok := val.([]byte)
	if !ok {
		t.Fatalf("Value returned %T, want []byte", val)
	}

	// Verify osm entry doesn't emit matched_at for zero time
	var tempMap map[string]map[string]interface{}
	if err := json.Unmarshal(b, &tempMap); err != nil {
		t.Fatalf("unmarshal for validation: %v", err)
	}
	if _, hasMatchedAt := tempMap["osm"]["matched_at"]; hasMatchedAt {
		t.Errorf("osm entry should not emit matched_at for zero time")
	}

	var got models.ExternalIDs
	if err := got.Scan(b); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for key, wantRef := range want {
		gotRef, ok := got[key]
		if !ok {
			t.Errorf("key %q missing after round-trip", key)
			continue
		}
		if gotRef.ID != wantRef.ID {
			t.Errorf("[%q].ID = %q, want %q", key, gotRef.ID, wantRef.ID)
		}
		if gotRef.Confidence != wantRef.Confidence {
			t.Errorf("[%q].Confidence = %v, want %v", key, gotRef.Confidence, wantRef.Confidence)
		}
		if !gotRef.MatchedAt.Equal(wantRef.MatchedAt) {
			t.Errorf("[%q].MatchedAt = %v, want %v", key, gotRef.MatchedAt, wantRef.MatchedAt)
		}
	}
}

func TestExternalIDs_Scan_Nil(t *testing.T) {
	var e models.ExternalIDs
	if err := e.Scan(nil); err != nil {
		t.Fatalf("Scan(nil): %v", err)
	}
	if len(e) != 0 {
		t.Errorf("len after nil scan = %d, want 0", len(e))
	}
}

func TestExternalIDs_Value_Nil(t *testing.T) {
	var e models.ExternalIDs
	val, err := e.Value()
	if err != nil {
		t.Fatalf("Value on nil map: %v", err)
	}
	b, ok := val.([]byte)
	if !ok {
		t.Fatalf("Value on nil returned %T, want []byte", val)
	}
	if string(b) != "{}" {
		t.Errorf("Value on nil = %q, want {}", string(b))
	}
}
