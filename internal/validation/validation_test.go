/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package validation

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	"github.com/InWheelOrg/inwheel-server/pkg/models"
)

func b64Enc(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func ptrFloat(v float64) *float64 { return &v }
func ptrInt(v int) *int            { return &v }

func validPlace() *models.Place {
	return &models.Place{
		Name:     "Test Cafe",
		Lat:      52.5,
		Lng:      13.4,
		Category: models.CategoryCafe,
	}
}

func errorsHaveField(errs []FieldError, field string) bool {
	for _, e := range errs {
		if e.Field == field {
			return true
		}
	}
	return false
}

// ── Email ─────────────────────────────────────────────────────────────────────

func TestEmail(t *testing.T) {
	valid := []string{
		"user@example.com",
		"first.last+tag@sub.example.co",
		"a_b-c@example.io",
	}
	for _, e := range valid {
		if errs := Email(e); len(errs) != 0 {
			t.Errorf("Email(%q) = %+v, want no errors", e, errs)
		}
	}

	invalid := []string{"", "no-at-sign", "user@", "@example.com", "user@example", "user@.com"}
	for _, e := range invalid {
		errs := Email(e)
		if len(errs) == 0 || errs[0].Field != "email" {
			t.Errorf("Email(%q) = %+v, want error on 'email'", e, errs)
		}
	}
}

// ── Place ─────────────────────────────────────────────────────────────────────

func TestPlace_Valid(t *testing.T) {
	if errs := Place(validPlace()); len(errs) != 0 {
		t.Errorf("expected no errors, got %+v", errs)
	}
}

func TestPlace_WhitespaceNameRejected(t *testing.T) {
	p := validPlace()
	p.Name = "   "
	if !errorsHaveField(Place(p), "name") {
		t.Error("expected error on whitespace-only name")
	}
}

func TestPlace_Tags(t *testing.T) {
	t.Run("too many entries", func(t *testing.T) {
		p := validPlace()
		p.Tags = make(models.PlaceTags, maxTagEntries+1)
		for i := 0; i < maxTagEntries+1; i++ {
			p.Tags[strings.Repeat("k", 1)+string(rune('a'+i%26))+string(rune('A'+i/26))] = "v"
		}
		if !errorsHaveField(Place(p), "tags") {
			t.Error("expected error on tags")
		}
	})
	t.Run("oversized value", func(t *testing.T) {
		p := validPlace()
		p.Tags = models.PlaceTags{"k": strings.Repeat("v", maxTagValueLength+1)}
		if !errorsHaveField(Place(p), "tags") {
			t.Error("expected error on tags")
		}
	})
	t.Run("oversized key", func(t *testing.T) {
		p := validPlace()
		p.Tags = models.PlaceTags{strings.Repeat("k", maxTagKeyLength+1): "v"}
		if !errorsHaveField(Place(p), "tags") {
			t.Error("expected error on tags")
		}
	})
}

// ── PlacesQuery ───────────────────────────────────────────────────────────────

// Mutual exclusivity and group completeness — these cannot be expressed in the
// OpenAPI spec and are the only query-param checks remaining in Go.
func TestPlacesQuery_GroupRules(t *testing.T) {
	tests := []struct {
		name  string
		query string
		valid bool
	}{
		{"empty query", "", true},
		{"full proximity group", "lng=13.4&lat=52.5&radius=500", true},
		{"full bbox group", "min_lng=13.0&min_lat=52.0&max_lng=14.0&max_lat=53.0", true},
		{"partial proximity — missing radius", "lng=13.4&lat=52.5", false},
		{"partial proximity — missing lat", "lng=13.4&radius=500", false},
		{"partial bbox — missing max_lat", "min_lng=13.0&min_lat=52.0&max_lng=14.0", false},
		{"both groups present", "lng=13.4&lat=52.5&radius=500&min_lng=13.0&min_lat=52.0&max_lng=14.0&max_lat=53.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, _ := url.ParseQuery(tt.query)
			errs := PlacesQuery(q)
			if (len(errs) == 0) != tt.valid {
				t.Errorf("query=%q valid=%v, want %v; errs=%+v", tt.query, len(errs) == 0, tt.valid, errs)
			}
		})
	}
}

// Bounding-box ordering: min must be strictly less than max.
func TestPlacesQuery_BBoxOrdering(t *testing.T) {
	t.Run("swapped lng", func(t *testing.T) {
		q, _ := url.ParseQuery("min_lng=14.0&min_lat=52.0&max_lng=13.0&max_lat=53.0")
		if !errorsHaveField(PlacesQuery(q), "min_lng") {
			t.Error("expected error when min_lng >= max_lng")
		}
	})
	t.Run("swapped lat", func(t *testing.T) {
		q, _ := url.ParseQuery("min_lng=13.0&min_lat=53.0&max_lng=14.0&max_lat=52.0")
		if !errorsHaveField(PlacesQuery(q), "min_lat") {
			t.Error("expected error when min_lat >= max_lat")
		}
	})
	t.Run("valid ordering", func(t *testing.T) {
		q, _ := url.ParseQuery("min_lng=13.0&min_lat=52.0&max_lng=14.0&max_lat=53.0")
		if errs := PlacesQuery(q); len(errs) != 0 {
			t.Errorf("expected no errors, got %+v", errs)
		}
	})
}

// Cursor format: base64-encoded timestamp|UUID pair.
func TestPlacesQuery_CursorParam(t *testing.T) {
	const validTS = "2026-05-01T12:00:00Z"
	const validID = "11111111-2222-3333-4444-555555555555"

	b64 := func(s string) string { return b64Enc([]byte(s)) }

	tests := []struct {
		name    string
		cursor  string
		wantErr bool
	}{
		{"no cursor", "", false},
		{"invalid base64", "!!!!", true},
		{"no pipe separator", b64("no-pipe-here"), true},
		{"bad timestamp", b64("not-a-time|" + validID), true},
		{"bad uuid", b64(validTS + "|not-a-uuid"), true},
		{"valid cursor", b64(validTS + "|" + validID), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawQuery := ""
			if tt.cursor != "" {
				rawQuery = "cursor=" + url.QueryEscape(tt.cursor)
			}
			q, _ := url.ParseQuery(rawQuery)
			errs := PlacesQuery(q)
			if tt.wantErr && !errorsHaveField(errs, "cursor") {
				t.Errorf("expected cursor error, got %+v", errs)
			}
			if !tt.wantErr && errorsHaveField(errs, "cursor") {
				t.Errorf("unexpected cursor error, got %+v", errs)
			}
		})
	}
}
