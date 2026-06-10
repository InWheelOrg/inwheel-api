/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package osm

import (
	"testing"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

func TestMapTagsToProfile_ReturnsNilWhenNoA11ySignal(t *testing.T) {
	got := mapTagsToProfile(map[string]string{
		"amenity": "cafe",
		"name":    "Café Pascal",
	})
	if got != nil {
		t.Errorf("expected nil profile when no a11y tags present, got %+v", got)
	}
}

func TestMapTagsToProfile_WheelchairValues(t *testing.T) {
	cases := []struct {
		tag  string
		want models.A11yStatus
	}{
		{"yes", models.StatusAccessible},
		{"designated", models.StatusAccessible},
		{"limited", models.StatusLimited},
		{"no", models.StatusInaccessible},
	}
	for _, c := range cases {
		t.Run(c.tag, func(t *testing.T) {
			got := mapTagsToProfile(map[string]string{"wheelchair": c.tag})
			if got == nil {
				t.Fatalf("expected profile, got nil")
			}
			if got.OverallStatus != c.want {
				t.Errorf("OverallStatus = %q, want %q", got.OverallStatus, c.want)
			}
		})
	}
}

func TestMapTagsToProfile_UnknownStatusWhenComponentOnlyTags(t *testing.T) {
	got := mapTagsToProfile(map[string]string{
		"toilets:wheelchair": "yes",
	})
	if got == nil {
		t.Fatalf("expected profile when toilet tag present")
	}
	if got.OverallStatus != models.StatusUnknown {
		t.Errorf("OverallStatus = %q, want unknown", got.OverallStatus)
	}
	if len(got.Components) != 1 || got.Components[0].Type != models.ComponentRestroom {
		t.Errorf("Components = %+v, want one restroom", got.Components)
	}
}

func TestMapTagsToProfile_Restroom(t *testing.T) {
	cases := []struct {
		name   string
		val    string
		want   models.A11yStatus
		access *bool
	}{
		{"yes", "yes", models.StatusAccessible, boolPtr(true)},
		{"no", "no", models.StatusInaccessible, boolPtr(false)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := mapTagsToProfile(map[string]string{"toilets:wheelchair": c.val})
			if got == nil {
				t.Fatalf("expected profile")
			}
			r := findComponent(t, got, models.ComponentRestroom)
			if r.OverallStatus != c.want {
				t.Errorf("OverallStatus = %q, want %q", r.OverallStatus, c.want)
			}
			if r.Restroom == nil || r.Restroom.WheelchairAccessible == nil {
				t.Fatalf("Restroom.WheelchairAccessible nil")
			}
			if *r.Restroom.WheelchairAccessible != *c.access {
				t.Errorf("WheelchairAccessible = %v, want %v", *r.Restroom.WheelchairAccessible, *c.access)
			}
		})
	}
}

func TestMapTagsToProfile_Parking(t *testing.T) {
	t.Run("capacity:disabled positive", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"capacity:disabled": "3"})
		p := findComponent(t, got, models.ComponentParking)
		if p.OverallStatus != models.StatusAccessible {
			t.Errorf("OverallStatus = %q, want accessible", p.OverallStatus)
		}
		if p.Parking == nil || p.Parking.HasDisabledSpaces == nil || !*p.Parking.HasDisabledSpaces {
			t.Errorf("HasDisabledSpaces not set true")
		}
		if p.Parking.Count == nil || *p.Parking.Count != 3 {
			t.Errorf("Count = %v, want 3", p.Parking.Count)
		}
	})
	t.Run("capacity:disabled zero", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"capacity:disabled": "0"})
		p := findComponent(t, got, models.ComponentParking)
		if p.OverallStatus != models.StatusInaccessible {
			t.Errorf("OverallStatus = %q, want inaccessible", p.OverallStatus)
		}
		if p.Parking == nil || p.Parking.HasDisabledSpaces == nil || *p.Parking.HasDisabledSpaces {
			t.Errorf("HasDisabledSpaces should be false")
		}
		if p.Parking.Count == nil || *p.Parking.Count != 0 {
			t.Errorf("Count = %v, want 0 (preserves confirmed-zero from source tag)", p.Parking.Count)
		}
	})
}

func TestMapTagsToProfile_Entrance(t *testing.T) {
	t.Run("automatic_door=button sets IsAutomatic", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"automatic_door": "button"})
		e := findComponent(t, got, models.ComponentEntrance)
		if e.Entrance == nil || e.Entrance.IsAutomatic == nil || !*e.Entrance.IsAutomatic {
			t.Errorf("IsAutomatic not set true")
		}
	})
	t.Run("automatic_door=no does not set IsAutomatic", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"automatic_door": "no"})
		if got != nil {
			t.Errorf("automatic_door=no alone should not emit an entrance component, got %+v", got)
		}
	})
	t.Run("step_count creates HasStep", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"step_count": "2"})
		e := findComponent(t, got, models.ComponentEntrance)
		if e.Entrance == nil || e.Entrance.HasStep == nil || !*e.Entrance.HasStep {
			t.Errorf("HasStep not set true")
		}
	})
	t.Run("entrance:step_count is honoured", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"entrance:step_count": "1"})
		e := findComponent(t, got, models.ComponentEntrance)
		if e.Entrance == nil || e.Entrance.HasStep == nil || !*e.Entrance.HasStep {
			t.Errorf("HasStep not set true via entrance:step_count")
		}
	})
	t.Run("ramp:wheelchair=yes sets HasRamp true", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"ramp:wheelchair": "yes"})
		e := findComponent(t, got, models.ComponentEntrance)
		if e.Entrance == nil || e.Entrance.HasRamp == nil || !*e.Entrance.HasRamp {
			t.Errorf("HasRamp not set true")
		}
	})
	t.Run("ramp:wheelchair=no sets HasRamp false", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"step_count": "1", "ramp:wheelchair": "no"})
		e := findComponent(t, got, models.ComponentEntrance)
		if e.Entrance == nil || e.Entrance.HasRamp == nil || *e.Entrance.HasRamp {
			t.Errorf("HasRamp should be false, got %v", e.Entrance.HasRamp)
		}
	})
	t.Run("ramp:wheelchair takes precedence over ramp", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"ramp": "yes", "ramp:wheelchair": "no", "step_count": "1"})
		e := findComponent(t, got, models.ComponentEntrance)
		if e.Entrance == nil || e.Entrance.HasRamp == nil || *e.Entrance.HasRamp {
			t.Errorf("ramp:wheelchair=no should win, got HasRamp=%v", e.Entrance.HasRamp)
		}
	})
	t.Run("ramp=yes alone returns nil (non-wheelchair-specific ramp is not a signal)", func(t *testing.T) {
		got := mapTagsToProfile(map[string]string{"ramp": "yes"})
		if got != nil {
			t.Errorf("ramp=yes alone should return nil, got %+v", got)
		}
	})
}

func TestMapTagsToProfile_Elevator(t *testing.T) {
	got := mapTagsToProfile(map[string]string{"elevator": "yes"})
	e := findComponent(t, got, models.ComponentElevator)
	if e.OverallStatus != models.StatusAccessible {
		t.Errorf("elevator status = %q, want accessible", e.OverallStatus)
	}
}

func TestMapTagsToProfile_AggregatesMultipleComponents(t *testing.T) {
	got := mapTagsToProfile(map[string]string{
		"wheelchair":         "yes",
		"toilets:wheelchair": "yes",
		"capacity:disabled":  "2",
		"automatic_door":     "yes",
		"elevator":           "yes",
	})
	if got == nil {
		t.Fatalf("expected profile")
	}
	if got.OverallStatus != models.StatusAccessible {
		t.Errorf("OverallStatus = %q, want accessible", got.OverallStatus)
	}
	want := map[models.A11yComponentType]bool{
		models.ComponentRestroom: false,
		models.ComponentParking:  false,
		models.ComponentEntrance: false,
		models.ComponentElevator: false,
	}
	for _, c := range got.Components {
		if _, ok := want[c.Type]; ok {
			want[c.Type] = true
		}
	}
	for k, v := range want {
		if !v {
			t.Errorf("missing %q component", k)
		}
	}
}

func findComponent(t *testing.T, p *models.AccessibilityProfile, ct models.A11yComponentType) models.A11yComponent {
	t.Helper()
	if p == nil {
		t.Fatalf("profile is nil")
	}
	for _, c := range p.Components {
		if c.Type == ct {
			return c
		}
	}
	t.Fatalf("component %q not found, components = %+v", ct, p.Components)
	return models.A11yComponent{}
}

func boolPtr(b bool) *bool { return &b }
