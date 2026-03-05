/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package a11y

import (
	"slices"

	"github.com/InWheelOrg/inwheel-server/pkg/models"
)

// Service provides logic for accessibility data processing and inheritance.
type Service struct{}

// ComputeEffectiveProfile merges accessibility components from a child place and its parent.
// Child places inherit parent components they don't own (e.g., a shop inherits a mall's parking).
// For any component taken from the parent, IsInherited is set to true and SourceID is set to parent.ID.
func (s *Service) ComputeEffectiveProfile(child, parent *models.Place) *models.AccessibilityProfile {
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
