/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package a11y

import (
	"testing"

	"github.com/InWheelOrg/inwheel-server/pkg/models"
)

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
			check: func(t *testing.T, res *models.AccessibilityProfile, parent *models.Place) {
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
