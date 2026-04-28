/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package validation

import (
	"math"
	"net/url"
	"strings"
	"testing"

	"github.com/InWheelOrg/inwheel-server/pkg/models"
)

func ptrFloat(v float64) *float64 { return &v }
func ptrInt(v int) *int            { return &v }
func ptrStr(v string) *string      { return &v }

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

func TestPlace_Valid(t *testing.T) {
	if errs := Place(validPlace()); len(errs) != 0 {
		t.Errorf("expected no errors, got %+v", errs)
	}
}

func TestPlace_Required(t *testing.T) {
	tests := []struct {
		name  string
		mut   func(p *models.Place)
		field string
	}{
		{"missing name", func(p *models.Place) { p.Name = "" }, "name"},
		{"whitespace name", func(p *models.Place) { p.Name = "   " }, "name"},
		{"missing category", func(p *models.Place) { p.Category = "" }, "category"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validPlace()
			tt.mut(p)
			errs := Place(p)
			if !errorsHaveField(errs, tt.field) {
				t.Errorf("expected error on field %q, got %+v", tt.field, errs)
			}
		})
	}
}

func TestPlace_Bounds(t *testing.T) {
	tests := []struct {
		name  string
		mut   func(p *models.Place)
		field string
	}{
		{"name too long", func(p *models.Place) { p.Name = strings.Repeat("a", maxNameLength+1) }, "name"},
		{"lat above max", func(p *models.Place) { p.Lat = 91 }, "lat"},
		{"lat below min", func(p *models.Place) { p.Lat = -91 }, "lat"},
		{"lat NaN", func(p *models.Place) { p.Lat = math.NaN() }, "lat"},
		{"lat Inf", func(p *models.Place) { p.Lat = math.Inf(1) }, "lat"},
		{"lng above max", func(p *models.Place) { p.Lng = 181 }, "lng"},
		{"lng below min", func(p *models.Place) { p.Lng = -181 }, "lng"},
		{"invalid category", func(p *models.Place) { p.Category = "spaceship" }, "category"},
		{"invalid rank", func(p *models.Place) { p.Rank = 4 }, "rank"},
		{"negative osm_id", func(p *models.Place) { p.OSMID = -1 }, "osm_id"},
		{"invalid osm_type", func(p *models.Place) { p.OSMType = "blob" }, "osm_type"},
		{"source too long", func(p *models.Place) { p.Source = strings.Repeat("s", maxSourceLength+1) }, "source"},
		{"invalid parent_id", func(p *models.Place) { p.ParentID = ptrStr("not-a-uuid") }, "parent_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validPlace()
			tt.mut(p)
			errs := Place(p)
			if !errorsHaveField(errs, tt.field) {
				t.Errorf("expected error on field %q, got %+v", tt.field, errs)
			}
		})
	}
}

func TestPlace_Edges(t *testing.T) {
	tests := []struct {
		name string
		mut  func(p *models.Place)
	}{
		{"lat at +90", func(p *models.Place) { p.Lat = 90 }},
		{"lat at -90", func(p *models.Place) { p.Lat = -90 }},
		{"lng at +180", func(p *models.Place) { p.Lng = 180 }},
		{"lng at -180", func(p *models.Place) { p.Lng = -180 }},
		{"name at max", func(p *models.Place) { p.Name = strings.Repeat("a", maxNameLength) }},
		{"valid parent_id", func(p *models.Place) { p.ParentID = ptrStr("11111111-1111-1111-1111-111111111111") }},
		{"valid rank=1", func(p *models.Place) { p.Rank = models.RankLandmark }},
		{"valid osm_type", func(p *models.Place) { p.OSMID = 12345; p.OSMType = models.OSMNode }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validPlace()
			tt.mut(p)
			if errs := Place(p); len(errs) != 0 {
				t.Errorf("expected no errors, got %+v", errs)
			}
		})
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
			t.Errorf("expected error on tags")
		}
	})
	t.Run("oversized value", func(t *testing.T) {
		p := validPlace()
		p.Tags = models.PlaceTags{"k": strings.Repeat("v", maxTagValueLength+1)}
		if !errorsHaveField(Place(p), "tags") {
			t.Errorf("expected error on tags")
		}
	})
	t.Run("oversized key", func(t *testing.T) {
		p := validPlace()
		p.Tags = models.PlaceTags{strings.Repeat("k", maxTagKeyLength+1): "v"}
		if !errorsHaveField(Place(p), "tags") {
			t.Errorf("expected error on tags")
		}
	})
}

func TestAccessibilityProfile_Required(t *testing.T) {
	errs := AccessibilityProfile(&models.AccessibilityProfile{})
	if !errorsHaveField(errs, "overall_status") {
		t.Errorf("expected error on overall_status, got %+v", errs)
	}
}

func TestAccessibilityProfile_InvalidStatus(t *testing.T) {
	prof := &models.AccessibilityProfile{OverallStatus: "questionable"}
	if !errorsHaveField(AccessibilityProfile(prof), "overall_status") {
		t.Errorf("expected error on overall_status")
	}
}

func TestAccessibilityProfile_ComponentRules(t *testing.T) {
	tests := []struct {
		name  string
		comp  models.A11yComponent
		field string
	}{
		{"missing type", models.A11yComponent{OverallStatus: models.StatusAccessible}, "components[0].type"},
		{"invalid type", models.A11yComponent{Type: "wormhole"}, "components[0].type"},
		{"invalid status", models.A11yComponent{Type: models.ComponentEntrance, OverallStatus: "?"}, "components[0].overall_status"},
		{"entrance width over max", models.A11yComponent{Type: models.ComponentEntrance, Entrance: &models.EntranceProperties{Width: ptrFloat(11)}}, "components[0].entrance.width"},
		{"entrance width negative", models.A11yComponent{Type: models.ComponentEntrance, Entrance: &models.EntranceProperties{Width: ptrFloat(-0.1)}}, "components[0].entrance.width"},
		{"entrance step_height over max", models.A11yComponent{Type: models.ComponentEntrance, Entrance: &models.EntranceProperties{StepHeight: ptrFloat(2)}}, "components[0].entrance.step_height"},
		{"restroom door over max", models.A11yComponent{Type: models.ComponentRestroom, Restroom: &models.RestroomProperties{DoorWidth: ptrFloat(11)}}, "components[0].restroom.door_width"},
		{"elevator width over max", models.A11yComponent{Type: models.ComponentElevator, Elevator: &models.ElevatorProperties{Width: ptrFloat(11)}}, "components[0].elevator.width"},
		{"elevator depth over max", models.A11yComponent{Type: models.ComponentElevator, Elevator: &models.ElevatorProperties{Depth: ptrFloat(11)}}, "components[0].elevator.depth"},
		{"parking count negative", models.A11yComponent{Type: models.ComponentParking, Parking: &models.ParkingProperties{Count: ptrInt(-1)}}, "components[0].parking.count"},
		{"parking count over max", models.A11yComponent{Type: models.ComponentParking, Parking: &models.ParkingProperties{Count: ptrInt(maxParkingCount + 1)}}, "components[0].parking.count"},
		{"entrance width NaN", models.A11yComponent{Type: models.ComponentEntrance, Entrance: &models.EntranceProperties{Width: ptrFloat(math.NaN())}}, "components[0].entrance.width"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prof := &models.AccessibilityProfile{
				OverallStatus: models.StatusAccessible,
				Components:    models.A11yComponents{tt.comp},
			}
			errs := AccessibilityProfile(prof)
			if !errorsHaveField(errs, tt.field) {
				t.Errorf("expected error on %q, got %+v", tt.field, errs)
			}
		})
	}
}

func TestPlaceID(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		valid bool
	}{
		{"valid", "11111111-1111-1111-1111-111111111111", true},
		{"empty", "", false},
		{"missing dashes", "11111111111111111111111111111111", false},
		{"non-hex", "ZZZZZZZZ-1111-1111-1111-111111111111", false},
		{"too short", "1111-1111-1111-1111-111111111111", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := PlaceID(tt.id)
			gotValid := len(errs) == 0
			if gotValid != tt.valid {
				t.Errorf("PlaceID(%q) valid = %v, want %v", tt.id, gotValid, tt.valid)
			}
		})
	}
}

func TestPlacesQuery(t *testing.T) {
	tests := []struct {
		name  string
		query string
		valid bool
	}{
		{"empty", "", true},
		{"valid circular", "lng=13.4&lat=52.5&radius=500", true},
		{"valid bbox", "min_lng=13.0&min_lat=52.0&max_lng=14.0&max_lat=53.0", true},
		{"partial circular", "lng=13.4&lat=52.5", false},
		{"partial bbox", "min_lng=13.0&min_lat=52.0&max_lng=14.0", false},
		{"both modes", "lng=13.4&lat=52.5&radius=500&min_lng=13.0&min_lat=52.0&max_lng=14.0&max_lat=53.0", false},
		{"radius zero", "lng=13.4&lat=52.5&radius=0", false},
		{"radius negative", "lng=13.4&lat=52.5&radius=-10", false},
		{"radius too large", "lng=13.4&lat=52.5&radius=99999", false},
		{"lat out of range", "lng=13.4&lat=91&radius=500", false},
		{"lng out of range", "lng=181&lat=52.5&radius=500", false},
		{"swapped bbox lng", "min_lng=14.0&min_lat=52.0&max_lng=13.0&max_lat=53.0", false},
		{"swapped bbox lat", "min_lng=13.0&min_lat=53.0&max_lng=14.0&max_lat=52.0", false},
		{"NaN circular", "lng=NaN&lat=52.5&radius=500", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, _ := url.ParseQuery(tt.query)
			errs := PlacesQuery(q)
			gotValid := len(errs) == 0
			if gotValid != tt.valid {
				t.Errorf("query=%q valid=%v, want %v; errs=%+v", tt.query, gotValid, tt.valid, errs)
			}
		})
	}
}
