/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package osm

import (
	"testing"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

func boolPtr(b bool) *bool { return &b }

func TestMapTagsToProfile_ReturnsNilWhenNoA11ySignal(t *testing.T) {
	t.Parallel()
	got := mapTagsToProfile(map[string]string{
		"amenity": "cafe",
		"name":    "Café Pascal",
	})
	if got != nil {
		t.Errorf("expected nil profile when no a11y tags present, got %+v", got)
	}
}

func TestMapTagsToProfile_WheelchairValues(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tag       string
		wantValue string
	}{
		{"yes", "yes"},
		{"designated", "designated"},
		{"limited", "limited"},
		{"no", "no"},
	}
	for _, c := range cases {
		t.Run(c.tag, func(t *testing.T) {
			got := mapTagsToProfile(map[string]string{"wheelchair": c.tag})
			if got == nil {
				t.Fatalf("expected profile, got nil")
			}
			if len(got.SourceReports) != 1 {
				t.Fatalf("expected 1 source report, got %d", len(got.SourceReports))
			}
			sr := got.SourceReports[0]
			if sr.Source != "osm" {
				t.Errorf("Source = %q, want osm", sr.Source)
			}
			if sr.Value != c.wantValue {
				t.Errorf("Value = %q, want %q", sr.Value, c.wantValue)
			}
		})
	}
}

func TestMapTagsToProfile_UnknownWheelchairTagSkipped(t *testing.T) {
	t.Parallel()
	got := mapTagsToProfile(map[string]string{"wheelchair": "permissive"})
	if got != nil {
		t.Errorf("unknown wheelchair value should not emit a profile, got %+v", got)
	}
}

func TestMapTagsToProfile_ComponentOnlyNoSourceReport(t *testing.T) {
	t.Parallel()
	got := mapTagsToProfile(map[string]string{"toilets:wheelchair": "yes"})
	if got == nil {
		t.Fatalf("expected profile when toilet tag present")
	}
	if len(got.SourceReports) != 0 {
		t.Errorf("expected no source reports, got %v", got.SourceReports)
	}
	if got.Restroom == nil {
		t.Error("expected Restroom to be set")
	}
}

func TestMapTagsToProfile_Restroom(t *testing.T) {
	t.Parallel()
	t.Run("yes sets IsAccessible true", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"toilets:wheelchair": "yes"})
		if got == nil || got.Restroom == nil {
			t.Fatal("expected restroom")
		}
		if got.Restroom.IsAccessible == nil || !*got.Restroom.IsAccessible {
			t.Error("IsAccessible should be true")
		}
	})
	t.Run("no sets IsAccessible false", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"toilets:wheelchair": "no"})
		if got == nil || got.Restroom == nil {
			t.Fatal("expected restroom")
		}
		if got.Restroom.IsAccessible == nil || *got.Restroom.IsAccessible {
			t.Error("IsAccessible should be false")
		}
	})
}

func TestMapTagsToProfile_Parking(t *testing.T) {
	t.Parallel()
	t.Run("capacity:disabled positive", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"capacity:disabled": "3"})
		if got == nil || got.Parking == nil {
			t.Fatal("expected parking")
		}
		if got.Parking.HasDisabledSpaces == nil || !*got.Parking.HasDisabledSpaces {
			t.Error("HasDisabledSpaces should be true")
		}
		if got.Parking.Count == nil || *got.Parking.Count != 3 {
			t.Errorf("Count = %v, want 3", got.Parking.Count)
		}
	})
	t.Run("capacity:disabled zero", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"capacity:disabled": "0"})
		if got == nil || got.Parking == nil {
			t.Fatal("expected parking")
		}
		if got.Parking.HasDisabledSpaces == nil || *got.Parking.HasDisabledSpaces {
			t.Error("HasDisabledSpaces should be false")
		}
		if got.Parking.Count == nil || *got.Parking.Count != 0 {
			t.Errorf("Count = %v, want 0", got.Parking.Count)
		}
	})
	t.Run("parking:disabled=no", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"parking:disabled": "no"})
		if got == nil || got.Parking == nil {
			t.Fatal("expected parking")
		}
		if got.Parking.HasDisabledSpaces == nil || *got.Parking.HasDisabledSpaces {
			t.Error("HasDisabledSpaces should be false")
		}
	})
}

func TestMapTagsToProfile_Entrance(t *testing.T) {
	t.Parallel()
	t.Run("automatic_door=button sets door type", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"automatic_door": "button"})
		if got == nil || got.Entrance == nil {
			t.Fatal("expected entrance")
		}
		if got.Entrance.Door == nil || got.Entrance.Door.Type != models.DoorAutomatic {
			t.Errorf("Door.Type = %q, want automatic", got.Entrance.Door.Type)
		}
	})
	t.Run("automatic_door=no does not produce entrance", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"automatic_door": "no"})
		if got != nil {
			t.Errorf("automatic_door=no alone should not emit a profile")
		}
	})
	t.Run("step_count sets IsLevel false", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"step_count": "2"})
		if got == nil || got.Entrance == nil {
			t.Fatal("expected entrance")
		}
		if got.Entrance.IsLevel == nil || *got.Entrance.IsLevel {
			t.Error("IsLevel should be false")
		}
	})
	t.Run("entrance:step_count is honoured", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"entrance:step_count": "1"})
		if got == nil || got.Entrance == nil {
			t.Fatal("expected entrance")
		}
		if got.Entrance.IsLevel == nil || *got.Entrance.IsLevel {
			t.Error("IsLevel should be false")
		}
	})
	t.Run("ramp:wheelchair=yes sets HasFixedRamp true", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"ramp:wheelchair": "yes"})
		if got == nil || got.Entrance == nil {
			t.Fatal("expected entrance")
		}
		if got.Entrance.HasFixedRamp == nil || !*got.Entrance.HasFixedRamp {
			t.Error("HasFixedRamp should be true")
		}
	})
	t.Run("ramp:wheelchair=no sets HasFixedRamp false", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"step_count": "1", "ramp:wheelchair": "no"})
		if got == nil || got.Entrance == nil {
			t.Fatal("expected entrance")
		}
		if got.Entrance.HasFixedRamp == nil || *got.Entrance.HasFixedRamp {
			t.Errorf("HasFixedRamp should be false, got %v", got.Entrance.HasFixedRamp)
		}
	})
	t.Run("ramp:wheelchair takes precedence over ramp", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"ramp": "yes", "ramp:wheelchair": "no", "step_count": "1"})
		if got == nil || got.Entrance == nil {
			t.Fatal("expected entrance")
		}
		if got.Entrance.HasFixedRamp == nil || *got.Entrance.HasFixedRamp {
			t.Errorf("ramp:wheelchair=no should win, HasFixedRamp=%v", got.Entrance.HasFixedRamp)
		}
	})
	t.Run("ramp=yes alone returns nil", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"ramp": "yes"})
		if got != nil {
			t.Errorf("ramp=yes alone should return nil, got %+v", got)
		}
	})
	t.Run("width sets Width level", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"width": "0.9"})
		if got == nil || got.Entrance == nil {
			t.Fatal("expected entrance")
		}
		if got.Entrance.Width == nil || *got.Entrance.Width != models.LevelGood {
			t.Errorf("Width = %v, want good", got.Entrance.Width)
		}
	})
	t.Run("width takes precedence over door:width", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"width": "0.9", "door:width": "0.6"})
		if got == nil || got.Entrance == nil {
			t.Fatal("expected entrance")
		}
		if got.Entrance.Width == nil || *got.Entrance.Width != models.LevelGood {
			t.Errorf("Width = %v, want good (width tag wins when present)", got.Entrance.Width)
		}
	})
	t.Run("door:width used when width absent", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"door:width": "0.6"})
		if got == nil || got.Entrance == nil {
			t.Fatal("expected entrance")
		}
		if got.Entrance.Width == nil || *got.Entrance.Width != models.LevelNo {
			t.Errorf("Width = %v, want no", got.Entrance.Width)
		}
	})
	t.Run("unparseable width is skipped", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"width": "narrow"})
		if got != nil {
			t.Errorf("unparseable width alone should return nil, got %+v", got)
		}
	})
	t.Run("incline sets SlopePercent level", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"incline": "10%"})
		if got == nil || got.Entrance == nil {
			t.Fatal("expected entrance")
		}
		if got.Entrance.SlopePercent == nil || *got.Entrance.SlopePercent != models.LevelNo {
			t.Errorf("SlopePercent = %v, want no", got.Entrance.SlopePercent)
		}
	})
	t.Run("negative incline is treated as its magnitude", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"incline": "-5%"})
		if got == nil || got.Entrance == nil {
			t.Fatal("expected entrance")
		}
		if got.Entrance.SlopePercent == nil || *got.Entrance.SlopePercent != models.LevelGood {
			t.Errorf("SlopePercent = %v, want good", got.Entrance.SlopePercent)
		}
	})
	t.Run("non-percentage incline is skipped", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"incline": "up"})
		if got != nil {
			t.Errorf("non-percentage incline alone should return nil, got %+v", got)
		}
	})
}

func TestMapTagsToProfile_Elevator(t *testing.T) {
	t.Parallel()
	got := mapTagsToProfile(map[string]string{"elevator": "yes"})
	if got == nil || got.Elevator == nil {
		t.Fatal("expected elevator")
	}
}

func TestMapTagsToProfile_AggregatesAllComponents(t *testing.T) {
	t.Parallel()
	got := mapTagsToProfile(map[string]string{
		"wheelchair":         "yes",
		"toilets:wheelchair": "yes",
		"capacity:disabled":  "2",
		"automatic_door":     "yes",
		"elevator":           "yes",
	})
	if got == nil {
		t.Fatal("expected profile")
	}
	if len(got.SourceReports) != 1 || got.SourceReports[0].Value != "yes" {
		t.Errorf("expected one OSM source report with value 'yes', got %v", got.SourceReports)
	}
	if got.Restroom == nil {
		t.Error("expected Restroom")
	}
	if got.Parking == nil {
		t.Error("expected Parking")
	}
	if got.Entrance == nil {
		t.Error("expected Entrance")
	}
	if got.Elevator == nil {
		t.Error("expected Elevator")
	}
}
