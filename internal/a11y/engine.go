/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package a11y

import (
	"slices"

	"github.com/InWheelOrg/inwheel-server/pkg/models"
)

// Audit flag constants for each technical violation detected by WithAuditFlags.
const (
	FlagEntranceNarrowWidth  = "narrow width (0.8m required)"
	FlagEntranceContainsStep = "contains step"
	FlagEntranceHighStep     = "high step (>0.05m)"
	FlagEntranceStepNoRamp   = "step with no ramp"

	FlagRestroomNotAccessible = "not wheelchair accessible"
	FlagRestroomNarrowDoor    = "narrow door (0.8m required)"
	FlagRestroomNoGrabRails   = "missing grab rails"

	FlagElevatorNarrowWidth = "small cabin width (0.8m required)"
	FlagElevatorShallowDep  = "small cabin depth (1.1m required)"
	FlagElevatorNoBraille   = "missing braille"
	FlagElevatorNoAudio     = "missing audio"

	FlagParkingNoDisabledSpaces = "no disabled spaces"
)

// Engine provides logic for accessibility data processing and inheritance.
type Engine struct{}

// ComputeEffectiveProfile merges accessibility components from a child place and its parent.
// Child places inherit parent components they don't own (e.g., a shop inherits a mall's parking).
// For any component taken from the parent, IsInherited is set to true and SourceID is set to parent.ID.
func (s *Engine) ComputeEffectiveProfile(child, parent *models.Place) *models.AccessibilityProfile {
	if child == nil {
		return nil
	}

	childCount := 0
	if child.Accessibility != nil {
		childCount = len(child.Accessibility.Components)
	}

	parentCount := 0
	if parent != nil && parent.Accessibility != nil {
		parentCount = len(parent.Accessibility.Components)
	}

	effective := &models.AccessibilityProfile{
		OverallStatus: models.StatusUnknown,
		Components:    make([]models.A11yComponent, 0, childCount+parentCount),
	}

	if child.Accessibility != nil {
		effective.OverallStatus = child.Accessibility.OverallStatus
		effective.Components = append(effective.Components, child.Accessibility.Components...)
	}

	if parent == nil || parent.Accessibility == nil {
		return effective
	}

	// Iterate through parent components and inherit those the child doesn't have.
	for _, pc := range parent.Accessibility.Components {
		if !hasComponent(child, pc.Type) {
			inherited := pc
			inherited.IsInherited = true
			inherited.SourceID = parent.ID
			effective.Components = append(effective.Components, inherited)
		}
	}

	return effective
}

// hasComponent checks if a place already has a component of a specific type.
func hasComponent(place *models.Place, cType models.A11yComponentType) bool {
	if place == nil || place.Accessibility == nil {
		return false
	}
	return slices.ContainsFunc(place.Accessibility.Components, func(c models.A11yComponent) bool {
		return c.Type == cType
	})
}

// WithAuditFlags performs a technical validation of each component and populates the AuditFlags field.
func (s *Engine) WithAuditFlags(profile *models.AccessibilityProfile) {
	if profile == nil {
		return
	}

	for i := range profile.Components {
		comp := &profile.Components[i]
		comp.AuditFlags = nil // Reset

		switch comp.Type {
		case models.ComponentEntrance:
			if e := comp.Entrance; e != nil {
				if e.Width != nil && *e.Width < 0.8 {
					comp.AuditFlags = append(comp.AuditFlags, FlagEntranceNarrowWidth)
				}
				if e.HasStep != nil && *e.HasStep {
					comp.AuditFlags = append(comp.AuditFlags, FlagEntranceContainsStep)
					if e.StepHeight != nil && *e.StepHeight > 0.05 {
						comp.AuditFlags = append(comp.AuditFlags, FlagEntranceHighStep)
					}
					if e.HasRamp != nil && !*e.HasRamp {
						comp.AuditFlags = append(comp.AuditFlags, FlagEntranceStepNoRamp)
					}
				}
			}
		case models.ComponentRestroom:
			if r := comp.Restroom; r != nil {
				if r.WheelchairAccessible != nil && !*r.WheelchairAccessible {
					comp.AuditFlags = append(comp.AuditFlags, FlagRestroomNotAccessible)
				}
				if r.DoorWidth != nil && *r.DoorWidth < 0.8 {
					comp.AuditFlags = append(comp.AuditFlags, FlagRestroomNarrowDoor)
				}
				if r.GrabRails != nil && !*r.GrabRails {
					comp.AuditFlags = append(comp.AuditFlags, FlagRestroomNoGrabRails)
				}
			}
		case models.ComponentElevator:
			if el := comp.Elevator; el != nil {
				if el.Width != nil && *el.Width < 0.8 {
					comp.AuditFlags = append(comp.AuditFlags, FlagElevatorNarrowWidth)
				}
				if el.Depth != nil && *el.Depth < 1.1 {
					comp.AuditFlags = append(comp.AuditFlags, FlagElevatorShallowDep)
				}
				if el.Braille != nil && !*el.Braille {
					comp.AuditFlags = append(comp.AuditFlags, FlagElevatorNoBraille)
				}
				if el.Audio != nil && !*el.Audio {
					comp.AuditFlags = append(comp.AuditFlags, FlagElevatorNoAudio)
				}
			}
		case models.ComponentParking:
			if p := comp.Parking; p != nil {
				if p.HasDisabledSpaces != nil && !*p.HasDisabledSpaces {
					comp.AuditFlags = append(comp.AuditFlags, FlagParkingNoDisabledSpaces)
				}
			}
		}
	}
}
