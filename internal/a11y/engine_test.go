/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package a11y

import (
	"slices"
	"testing"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

func boolPtr(v bool) *bool        { return &v }
func floatPtr(v float64) *float64 { return &v }

func TestComputeEffectiveProfile(t *testing.T) {
	t.Parallel()
	engine := &Engine{}

	t.Run("nil child returns nil", func(t *testing.T) {
		if engine.ComputeEffectiveProfile(nil, nil) != nil {
			t.Error("expected nil for nil child")
		}
	})

	t.Run("child with no accessibility and no parent", func(t *testing.T) {
		res := engine.ComputeEffectiveProfile(&models.Place{ID: "c"}, nil)
		if res == nil {
			t.Fatal("expected non-nil profile")
		}
		if res.Entrance != nil || res.Parking != nil || res.Restroom != nil {
			t.Error("expected all components nil for place with no accessibility")
		}
	})

	t.Run("child inherits parent parking when child has none", func(t *testing.T) {
		parent := &models.Place{
			ID: "parent-1",
			Accessibility: &models.AccessibilityProfile{
				Parking: &models.ParkingProps{HasDisabledSpaces: boolPtr(true)},
			},
		}
		child := &models.Place{
			ID: "child-1",
			Accessibility: &models.AccessibilityProfile{
				Entrance: &models.EntranceProps{IsLevel: boolPtr(true)},
			},
		}
		res := engine.ComputeEffectiveProfile(child, parent)

		if res.Entrance == nil || res.Entrance.IsInherited {
			t.Error("child's own entrance should be present and not inherited")
		}
		if res.Parking == nil {
			t.Fatal("parent parking should be inherited")
		}
		if !res.Parking.IsInherited {
			t.Error("inherited parking should have IsInherited=true")
		}
		if res.Parking.SourceID != parent.ID {
			t.Errorf("inherited parking SourceID = %q, want %q", res.Parking.SourceID, parent.ID)
		}
	})

	t.Run("child entrance overrides parent entrance", func(t *testing.T) {
		parent := &models.Place{
			ID: "parent-1",
			Accessibility: &models.AccessibilityProfile{
				Entrance: &models.EntranceProps{IsLevel: boolPtr(true)},
			},
		}
		child := &models.Place{
			ID: "child-1",
			Accessibility: &models.AccessibilityProfile{
				Entrance: &models.EntranceProps{IsLevel: boolPtr(false)},
			},
		}
		res := engine.ComputeEffectiveProfile(child, parent)

		if res.Entrance == nil {
			t.Fatal("expected entrance")
		}
		if res.Entrance.IsInherited {
			t.Error("child's own entrance should not be inherited")
		}
		if res.Entrance.IsLevel == nil || *res.Entrance.IsLevel {
			t.Error("expected child's is_level=false to override parent's is_level=true")
		}
	})

	t.Run("original child and parent records are not mutated", func(t *testing.T) {
		parent := &models.Place{
			ID: "p",
			Accessibility: &models.AccessibilityProfile{
				Parking: &models.ParkingProps{HasDisabledSpaces: boolPtr(true)},
			},
		}
		child := &models.Place{ID: "c", Accessibility: &models.AccessibilityProfile{}}

		engine.ComputeEffectiveProfile(child, parent)

		if parent.Accessibility.Parking.IsInherited {
			t.Error("parent parking should not be mutated")
		}
	})
}

func TestWithAuditFlags(t *testing.T) {
	t.Parallel()
	engine := &Engine{}

	t.Run("nil profile does not panic", func(_ *testing.T) {
		engine.WithAuditFlags(nil)
	})

	tests := []struct {
		name      string
		profile   models.AccessibilityProfile
		wantFlags map[string][]string // component → expected flags
	}{
		// --- entrance ---
		{
			name:      "entrance: no properties, no flags",
			profile:   models.AccessibilityProfile{Entrance: &models.EntranceProps{}},
			wantFlags: map[string][]string{"entrance": nil},
		},
		{
			name:      "entrance: width below 0.8m",
			profile:   models.AccessibilityProfile{Entrance: &models.EntranceProps{Width: floatPtr(0.75)}},
			wantFlags: map[string][]string{"entrance": {FlagEntranceNarrowWidth}},
		},
		{
			name:      "entrance: width exactly 0.8m, no flag",
			profile:   models.AccessibilityProfile{Entrance: &models.EntranceProps{Width: floatPtr(0.8)}},
			wantFlags: map[string][]string{"entrance": nil},
		},
		{
			name:      "entrance: not level and no ramp",
			profile:   models.AccessibilityProfile{Entrance: &models.EntranceProps{IsLevel: boolPtr(false)}},
			wantFlags: map[string][]string{"entrance": {FlagEntranceNoLevelRoute}},
		},
		{
			name:      "entrance: not level but has fixed ramp, no flag",
			profile:   models.AccessibilityProfile{Entrance: &models.EntranceProps{IsLevel: boolPtr(false), HasFixedRamp: boolPtr(true)}},
			wantFlags: map[string][]string{"entrance": nil},
		},
		{
			name:      "entrance: is level, no flag",
			profile:   models.AccessibilityProfile{Entrance: &models.EntranceProps{IsLevel: boolPtr(true)}},
			wantFlags: map[string][]string{"entrance": nil},
		},

		// --- pathways ---
		{
			name:      "pathway: width below 0.9m",
			profile:   models.AccessibilityProfile{Pathways: &models.PathwayProps{Width: floatPtr(0.8)}},
			wantFlags: map[string][]string{"pathways": {FlagPathwayNarrowWidth}},
		},
		{
			name:      "pathway: width at 0.9m, no flag",
			profile:   models.AccessibilityProfile{Pathways: &models.PathwayProps{Width: floatPtr(0.9)}},
			wantFlags: map[string][]string{"pathways": nil},
		},

		// --- restroom ---
		{
			name:      "restroom: door width below 0.8m",
			profile:   models.AccessibilityProfile{Restroom: &models.RestroomProps{DoorWidth: floatPtr(0.75)}},
			wantFlags: map[string][]string{"restroom": {FlagRestroomNarrowDoor}},
		},
		{
			name:      "restroom: turning radius below 1.5m",
			profile:   models.AccessibilityProfile{Restroom: &models.RestroomProps{TurningRadius: floatPtr(1.2)}},
			wantFlags: map[string][]string{"restroom": {FlagRestroomSmallTurning}},
		},
		{
			name:      "restroom: no grab rails",
			profile:   models.AccessibilityProfile{Restroom: &models.RestroomProps{HasGrabRails: boolPtr(false)}},
			wantFlags: map[string][]string{"restroom": {FlagRestroomNoGrabRails}},
		},
		{
			name:      "restroom: has grab rails, no flag",
			profile:   models.AccessibilityProfile{Restroom: &models.RestroomProps{HasGrabRails: boolPtr(true)}},
			wantFlags: map[string][]string{"restroom": nil},
		},

		// --- parking ---
		{
			name:      "parking: no disabled spaces",
			profile:   models.AccessibilityProfile{Parking: &models.ParkingProps{HasDisabledSpaces: boolPtr(false)}},
			wantFlags: map[string][]string{"parking": {FlagParkingNoDisabledSpaces}},
		},
		{
			name:      "parking: has disabled spaces, no flag",
			profile:   models.AccessibilityProfile{Parking: &models.ParkingProps{HasDisabledSpaces: boolPtr(true)}},
			wantFlags: map[string][]string{"parking": nil},
		},

		// --- elevator ---
		{
			name:      "elevator: width below 0.8m",
			profile:   models.AccessibilityProfile{Elevator: &models.ElevatorProps{Width: floatPtr(0.7)}},
			wantFlags: map[string][]string{"elevator": {FlagElevatorNarrowWidth}},
		},
		{
			name:      "elevator: depth below 1.1m",
			profile:   models.AccessibilityProfile{Elevator: &models.ElevatorProps{Depth: floatPtr(1.0)}},
			wantFlags: map[string][]string{"elevator": {FlagElevatorShallowDepth}},
		},
		{
			name:      "elevator: door width below 0.8m",
			profile:   models.AccessibilityProfile{Elevator: &models.ElevatorProps{DoorWidth: floatPtr(0.75)}},
			wantFlags: map[string][]string{"elevator": {FlagElevatorNarrowDoor}},
		},
		{
			name:      "elevator: no braille",
			profile:   models.AccessibilityProfile{Elevator: &models.ElevatorProps{HasBraille: boolPtr(false)}},
			wantFlags: map[string][]string{"elevator": {FlagElevatorNoBraille}},
		},
		{
			name:      "elevator: no audio",
			profile:   models.AccessibilityProfile{Elevator: &models.ElevatorProps{HasAudio: boolPtr(false)}},
			wantFlags: map[string][]string{"elevator": {FlagElevatorNoAudio}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.profile
			engine.WithAuditFlags(&p)

			checkFlags := func(component string, got []string, want []string) {
				if len(want) == 0 && len(got) == 0 {
					return
				}
				if len(got) != len(want) {
					t.Errorf("%s: flags = %v, want %v", component, got, want)
					return
				}
				for _, wf := range want {
					if !slices.Contains(got, wf) {
						t.Errorf("%s: missing flag %q in %v", component, wf, got)
					}
				}
			}

			for comp, want := range tt.wantFlags {
				switch comp {
				case "entrance":
					var got []string
					if p.Entrance != nil {
						got = p.Entrance.AuditFlags
					}
					checkFlags(comp, got, want)
				case "pathways":
					var got []string
					if p.Pathways != nil {
						got = p.Pathways.AuditFlags
					}
					checkFlags(comp, got, want)
				case "restroom":
					var got []string
					if p.Restroom != nil {
						got = p.Restroom.AuditFlags
					}
					checkFlags(comp, got, want)
				case "parking":
					var got []string
					if p.Parking != nil {
						got = p.Parking.AuditFlags
					}
					checkFlags(comp, got, want)
				case "elevator":
					var got []string
					if p.Elevator != nil {
						got = p.Elevator.AuditFlags
					}
					checkFlags(comp, got, want)
				}
			}
		})
	}
}
