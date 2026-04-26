/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package a11y

import (
	"slices"
	"testing"

	"github.com/InWheelOrg/inwheel-server/pkg/models"
)

func boolPtr(v bool) *bool        { return &v }
func floatPtr(v float64) *float64 { return &v }

func TestComputeEffectiveProfile(t *testing.T) {
	engine := &Engine{}

	tests := []struct {
		name          string
		child         *models.Place
		parent        *models.Place
		wantStatus    models.A11yStatus
		wantCompCount int
		check         func(t *testing.T, res *models.AccessibilityProfile, parent *models.Place)
	}{
		{
			name:       "nil child returns nil",
			child:      nil,
			parent:     nil,
			wantStatus: "", // nil expected
		},
		{
			name:          "child without accessibility and no parent",
			child:         &models.Place{ID: "child-1"},
			parent:        nil,
			wantStatus:    models.StatusUnknown,
			wantCompCount: 0,
		},
		{
			name: "child inherits from parent",
			parent: &models.Place{
				ID: "parent-1",
				Accessibility: &models.AccessibilityProfile{
					OverallStatus: models.StatusAccessible,
					Components: []models.A11yComponent{
						{
							Type:          models.ComponentParking,
							OverallStatus: models.StatusAccessible,
						},
					},
				},
			},
			child: &models.Place{
				ID: "child-1",
				Accessibility: &models.AccessibilityProfile{
					OverallStatus: models.StatusLimited,
					Components: []models.A11yComponent{
						{
							Type:          models.ComponentEntrance,
							OverallStatus: models.StatusLimited,
						},
					},
				},
			},
			wantStatus:    models.StatusLimited,
			wantCompCount: 2,
			check: func(t *testing.T, res *models.AccessibilityProfile, parent *models.Place) {
				// Check for child's own component
				var entranceFound bool
				for _, c := range res.Components {
					if c.Type == models.ComponentEntrance {
						entranceFound = true
						if c.IsInherited {
							t.Error("Child entrance component should not be marked as inherited")
						}
					}
				}
				if !entranceFound {
					t.Error("Child entrance component missing from effective profile")
				}

				// Check for inherited parent component
				var parkingFound bool
				for _, c := range res.Components {
					if c.Type == models.ComponentParking {
						parkingFound = true
						if !c.IsInherited {
							t.Error("Parent parking component should be marked as inherited")
						}
						if c.SourceID != parent.ID {
							t.Errorf("Expected SourceID %s, got %s", parent.ID, c.SourceID)
						}
					}
				}
				if !parkingFound {
					t.Error("Parent parking component missing from effective profile")
				}
			},
		},
		{
			name: "child component overrides parent component",
			parent: &models.Place{
				ID: "parent-1",
				Accessibility: &models.AccessibilityProfile{
					Components: []models.A11yComponent{
						{
							Type:          models.ComponentEntrance,
							OverallStatus: models.StatusAccessible,
						},
					},
				},
			},
			child: &models.Place{
				ID: "child-1",
				Accessibility: &models.AccessibilityProfile{
					OverallStatus: models.StatusUnknown,
					Components: []models.A11yComponent{
						{
							Type:          models.ComponentEntrance,
							OverallStatus: models.StatusInaccessible,
						},
					},
				},
			},
			wantStatus:    models.StatusUnknown,
			wantCompCount: 1,
			check: func(t *testing.T, res *models.AccessibilityProfile, _ *models.Place) {
				if res.Components[0].OverallStatus != models.StatusInaccessible {
					t.Errorf("Expected child status %s to override parent, got %s", models.StatusInaccessible, res.Components[0].OverallStatus)
				}
				if res.Components[0].IsInherited {
					t.Error("Child component should not be marked as inherited")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := engine.ComputeEffectiveProfile(tt.child, tt.parent)

			if tt.child == nil {
				if res != nil {
					t.Errorf("Expected nil for nil child, got %v", res)
				}
				return
			}

			if res == nil {
				t.Fatal("Expected non-nil profile")
			}

			if res.OverallStatus != tt.wantStatus {
				t.Errorf("OverallStatus = %s, want %s", res.OverallStatus, tt.wantStatus)
			}

			if len(res.Components) != tt.wantCompCount {
				t.Errorf("len(Components) = %d, want %d", len(res.Components), tt.wantCompCount)
			}

			if tt.check != nil {
				tt.check(t, res, tt.parent)
			}
		})
	}
}

func TestDetectConflicts(t *testing.T) {
	engine := &Engine{}

	tests := []struct {
		name          string
		components    []models.A11yComponent
		wantConflicts int
	}{
		{
			name:          "nil profile",
			components:    nil,
			wantConflicts: 0,
		},
		{
			name: "no components",
			components:    []models.A11yComponent{},
			wantConflicts: 0,
		},
		// Hard contradictions — must block
		{
			name: "entrance: step with no ramp + accessible",
			components: []models.A11yComponent{
				{
					Type:          models.ComponentEntrance,
					OverallStatus: models.StatusAccessible,
					AuditFlags:    []string{FlagEntranceStepNoRamp},
				},
			},
			wantConflicts: 1,
		},
		{
			name: "restroom: not wheelchair accessible + accessible",
			components: []models.A11yComponent{
				{
					Type:          models.ComponentRestroom,
					OverallStatus: models.StatusAccessible,
					AuditFlags:    []string{FlagRestroomNotAccessible},
				},
			},
			wantConflicts: 1,
		},
		{
			name: "parking: no disabled spaces + accessible",
			components: []models.A11yComponent{
				{
					Type:          models.ComponentParking,
					OverallStatus: models.StatusAccessible,
					AuditFlags:    []string{FlagParkingNoDisabledSpaces},
				},
			},
			wantConflicts: 1,
		},
		// Informational threshold flags — must not block
		{
			name: "entrance: narrow width + accessible — informational only",
			components: []models.A11yComponent{
				{
					Type:          models.ComponentEntrance,
					OverallStatus: models.StatusAccessible,
					AuditFlags:    []string{FlagEntranceNarrowWidth},
				},
			},
			wantConflicts: 0,
		},
		{
			name: "entrance: high step + accessible — informational only",
			components: []models.A11yComponent{
				{
					Type:          models.ComponentEntrance,
					OverallStatus: models.StatusAccessible,
					AuditFlags:    []string{FlagEntranceHighStep},
				},
			},
			wantConflicts: 0,
		},
		{
			name: "restroom: narrow door + accessible — informational only",
			components: []models.A11yComponent{
				{
					Type:          models.ComponentRestroom,
					OverallStatus: models.StatusAccessible,
					AuditFlags:    []string{FlagRestroomNarrowDoor},
				},
			},
			wantConflicts: 0,
		},
		{
			name: "elevator: narrow width + accessible — informational only",
			components: []models.A11yComponent{
				{
					Type:          models.ComponentElevator,
					OverallStatus: models.StatusAccessible,
					AuditFlags:    []string{FlagElevatorNarrowWidth},
				},
			},
			wantConflicts: 0,
		},
		// Status other than accessible — never a conflict
		{
			name: "step with no ramp + limited — no conflict",
			components: []models.A11yComponent{
				{
					Type:          models.ComponentEntrance,
					OverallStatus: models.StatusLimited,
					AuditFlags:    []string{FlagEntranceStepNoRamp},
				},
			},
			wantConflicts: 0,
		},
		{
			name: "restroom not accessible + inaccessible — no conflict",
			components: []models.A11yComponent{
				{
					Type:          models.ComponentRestroom,
					OverallStatus: models.StatusInaccessible,
					AuditFlags:    []string{FlagRestroomNotAccessible},
				},
			},
			wantConflicts: 0,
		},
		// Multiple components — only conflicting ones reported
		{
			name: "two components, one conflict",
			components: []models.A11yComponent{
				{
					Type:          models.ComponentEntrance,
					OverallStatus: models.StatusAccessible,
					AuditFlags:    []string{FlagEntranceNarrowWidth}, // informational only
				},
				{
					Type:          models.ComponentRestroom,
					OverallStatus: models.StatusAccessible,
					AuditFlags:    []string{FlagRestroomNotAccessible}, // hard conflict
				},
			},
			wantConflicts: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var profile *models.AccessibilityProfile
			if tt.components != nil {
				profile = &models.AccessibilityProfile{Components: tt.components}
			}
			conflicts := engine.DetectConflicts(profile)
			if len(conflicts) != tt.wantConflicts {
				t.Errorf("DetectConflicts() = %d conflicts, want %d: %v", len(conflicts), tt.wantConflicts, conflicts)
			}
		})
	}
}

func TestWithAuditFlags(t *testing.T) {
	engine := &Engine{}

	tests := []struct {
		name      string
		component models.A11yComponent
		wantFlags []string
	}{
		// --- nil / empty ---
		{
			name:      "nil profile does not panic",
			component: models.A11yComponent{}, // tested separately below via nil call
		},

		// --- entrance ---
		{
			name: "entrance: no properties set — no flags",
			component: models.A11yComponent{
				Type:     models.ComponentEntrance,
				Entrance: &models.EntranceProperties{},
			},
			wantFlags: nil,
		},
		{
			name: "entrance: width below minimum",
			component: models.A11yComponent{
				Type:     models.ComponentEntrance,
				Entrance: &models.EntranceProperties{Width: floatPtr(0.75)},
			},
			wantFlags: []string{FlagEntranceNarrowWidth},
		},
		{
			name: "entrance: width exactly at minimum — no flag",
			component: models.A11yComponent{
				Type:     models.ComponentEntrance,
				Entrance: &models.EntranceProperties{Width: floatPtr(0.8)},
			},
			wantFlags: nil,
		},
		{
			name: "entrance: has step",
			component: models.A11yComponent{
				Type:     models.ComponentEntrance,
				Entrance: &models.EntranceProperties{HasStep: boolPtr(true)},
			},
			wantFlags: []string{FlagEntranceContainsStep},
		},
		{
			name: "entrance: step height above threshold",
			component: models.A11yComponent{
				Type: models.ComponentEntrance,
				Entrance: &models.EntranceProperties{
					HasStep:    boolPtr(true),
					StepHeight: floatPtr(0.1),
				},
			},
			wantFlags: []string{FlagEntranceContainsStep, FlagEntranceHighStep},
		},
		{
			name: "entrance: step height at threshold — no high-step flag",
			component: models.A11yComponent{
				Type: models.ComponentEntrance,
				Entrance: &models.EntranceProperties{
					HasStep:    boolPtr(true),
					StepHeight: floatPtr(0.05),
				},
			},
			wantFlags: []string{FlagEntranceContainsStep},
		},
		{
			name: "entrance: step with no ramp",
			component: models.A11yComponent{
				Type: models.ComponentEntrance,
				Entrance: &models.EntranceProperties{
					HasStep: boolPtr(true),
					HasRamp: boolPtr(false),
				},
			},
			wantFlags: []string{FlagEntranceContainsStep, FlagEntranceStepNoRamp},
		},
		{
			name: "entrance: step with ramp — no ramp flag",
			component: models.A11yComponent{
				Type: models.ComponentEntrance,
				Entrance: &models.EntranceProperties{
					HasStep: boolPtr(true),
					HasRamp: boolPtr(true),
				},
			},
			wantFlags: []string{FlagEntranceContainsStep},
		},
		{
			name: "entrance: nil entrance — no flags",
			component: models.A11yComponent{
				Type:     models.ComponentEntrance,
				Entrance: nil,
			},
			wantFlags: nil,
		},

		// --- restroom ---
		{
			name: "restroom: not wheelchair accessible",
			component: models.A11yComponent{
				Type:     models.ComponentRestroom,
				Restroom: &models.RestroomProperties{WheelchairAccessible: boolPtr(false)},
			},
			wantFlags: []string{FlagRestroomNotAccessible},
		},
		{
			name: "restroom: door width below minimum",
			component: models.A11yComponent{
				Type:     models.ComponentRestroom,
				Restroom: &models.RestroomProperties{DoorWidth: floatPtr(0.7)},
			},
			wantFlags: []string{FlagRestroomNarrowDoor},
		},
		{
			name: "restroom: door width exactly at minimum — no flag",
			component: models.A11yComponent{
				Type:     models.ComponentRestroom,
				Restroom: &models.RestroomProperties{DoorWidth: floatPtr(0.8)},
			},
			wantFlags: nil,
		},
		{
			name: "restroom: missing grab rails",
			component: models.A11yComponent{
				Type:     models.ComponentRestroom,
				Restroom: &models.RestroomProperties{GrabRails: boolPtr(false)},
			},
			wantFlags: []string{FlagRestroomNoGrabRails},
		},
		{
			name: "restroom: nil restroom — no flags",
			component: models.A11yComponent{
				Type:     models.ComponentRestroom,
				Restroom: nil,
			},
			wantFlags: nil,
		},

		// --- elevator ---
		{
			name: "elevator: cabin width below minimum",
			component: models.A11yComponent{
				Type:     models.ComponentElevator,
				Elevator: &models.ElevatorProperties{Width: floatPtr(0.7)},
			},
			wantFlags: []string{FlagElevatorNarrowWidth},
		},
		{
			name: "elevator: cabin depth below minimum",
			component: models.A11yComponent{
				Type:     models.ComponentElevator,
				Elevator: &models.ElevatorProperties{Depth: floatPtr(1.0)},
			},
			wantFlags: []string{FlagElevatorShallowDep},
		},
		{
			name: "elevator: missing braille",
			component: models.A11yComponent{
				Type:     models.ComponentElevator,
				Elevator: &models.ElevatorProperties{Braille: boolPtr(false)},
			},
			wantFlags: []string{FlagElevatorNoBraille},
		},
		{
			name: "elevator: missing audio",
			component: models.A11yComponent{
				Type:     models.ComponentElevator,
				Elevator: &models.ElevatorProperties{Audio: boolPtr(false)},
			},
			wantFlags: []string{FlagElevatorNoAudio},
		},
		{
			name: "elevator: nil elevator — no flags",
			component: models.A11yComponent{
				Type:     models.ComponentElevator,
				Elevator: nil,
			},
			wantFlags: nil,
		},

		// --- parking ---
		{
			name: "parking: no disabled spaces",
			component: models.A11yComponent{
				Type:    models.ComponentParking,
				Parking: &models.ParkingProperties{HasDisabledSpaces: boolPtr(false)},
			},
			wantFlags: []string{FlagParkingNoDisabledSpaces},
		},
		{
			name: "parking: has disabled spaces — no flag",
			component: models.A11yComponent{
				Type:    models.ComponentParking,
				Parking: &models.ParkingProperties{HasDisabledSpaces: boolPtr(true)},
			},
			wantFlags: nil,
		},
		{
			name: "parking: nil parking — no flags",
			component: models.A11yComponent{
				Type:    models.ComponentParking,
				Parking: nil,
			},
			wantFlags: nil,
		},

		// --- other ---
		{
			name: "component type other — no flags regardless of data",
			component: models.A11yComponent{
				Type: models.ComponentOther,
			},
			wantFlags: nil,
		},
	}

	// Nil profile must not panic.
	t.Run("nil profile does not panic", func(_ *testing.T) {
		engine.WithAuditFlags(nil)
	})

	// Existing flags are cleared before re-evaluation.
	t.Run("existing flags are reset", func(t *testing.T) {
		profile := &models.AccessibilityProfile{
			Components: []models.A11yComponent{
				{
					Type:       models.ComponentEntrance,
					AuditFlags: []string{"stale flag"},
					Entrance:   &models.EntranceProperties{Width: floatPtr(1.0)},
				},
			},
		}
		engine.WithAuditFlags(profile)
		if len(profile.Components[0].AuditFlags) != 0 {
			t.Errorf("expected stale flags to be cleared, got %v", profile.Components[0].AuditFlags)
		}
	})

	for _, tt := range tests {
		if tt.name == "nil profile does not panic" {
			continue // handled above
		}
		t.Run(tt.name, func(t *testing.T) {
			profile := &models.AccessibilityProfile{
				Components: []models.A11yComponent{tt.component},
			}
			engine.WithAuditFlags(profile)
			got := profile.Components[0].AuditFlags

			if len(tt.wantFlags) == 0 && len(got) == 0 {
				return
			}
			if len(got) != len(tt.wantFlags) {
				t.Errorf("flags = %v, want %v", got, tt.wantFlags)
				return
			}
			for _, wf := range tt.wantFlags {
				if !slices.Contains(got, wf) {
					t.Errorf("missing expected flag %q in %v", wf, got)
				}
			}
		})
	}
}
