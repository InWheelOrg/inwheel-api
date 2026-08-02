/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

// Package a11y implements the accessibility rule engine: audit flag computation
// and parent-component inheritance. Runs synchronously on every profile write.
package a11y

import (
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// Audit flag constants computed from submitted property values.
const (
	FlagEntranceNarrowWidth  = "narrow width"
	FlagEntranceNoLevelRoute = "no level route (no ramp and not level)"

	FlagRestroomNarrowDoor   = "narrow door"
	FlagRestroomSmallTurning = "small turning radius"
	FlagRestroomNoGrabRails  = "missing grab rails"

	FlagElevatorNarrowWidth  = "small cabin width"
	FlagElevatorShallowDepth = "small cabin depth"
	FlagElevatorNarrowDoor   = "narrow door"
	FlagElevatorNoBraille    = "missing braille"
	FlagElevatorNoAudio      = "missing audio"

	FlagParkingNoDisabledSpaces = "no disabled spaces"

	FlagPathwayNarrowWidth = "narrow pathway"
)

// LevelForDoorWidth maps a clear opening width in metres to an AccessibilityLevel.
// Good >=0.80m, limited 0.70-0.80m, no <0.70m (SIA 500).
func LevelForDoorWidth(metres float64) models.AccessibilityLevel {
	switch {
	case metres >= 0.80:
		return models.LevelGood
	case metres >= 0.70:
		return models.LevelLimited
	default:
		return models.LevelNo
	}
}

// LevelForSlopePercent maps a ramp slope percentage to an AccessibilityLevel.
// Good <=6%, limited 6-8.33%, no >8.33% (ADA).
func LevelForSlopePercent(percent float64) models.AccessibilityLevel {
	switch {
	case percent <= 6:
		return models.LevelGood
	case percent <= 8.33:
		return models.LevelLimited
	default:
		return models.LevelNo
	}
}

type Engine struct{}

// WithAuditFlags computes AuditFlags on each non-nil component of the profile.
func (e *Engine) WithAuditFlags(profile *models.AccessibilityProfile) {
	if profile == nil {
		return
	}

	if profile.Entrance != nil {
		profile.Entrance.AuditFlags = entranceFlags(profile.Entrance)
	}
	if profile.Pathways != nil {
		profile.Pathways.AuditFlags = pathwayFlags(profile.Pathways)
	}
	if profile.Restroom != nil {
		profile.Restroom.AuditFlags = restroomFlags(profile.Restroom)
	}
	if profile.Parking != nil {
		profile.Parking.AuditFlags = parkingFlags(profile.Parking)
	}
	if profile.Elevator != nil {
		profile.Elevator.AuditFlags = elevatorFlags(profile.Elevator)
	}
}

// ComputeEffectiveProfile merges components from a child place and its parent.
// Child places inherit parent components they don't own (e.g. a shop inherits a
// mall's parking). Inherited components have IsInherited=true and SourceID=parent.ID.
// Neither the child nor parent record is mutated.
func (e *Engine) ComputeEffectiveProfile(child, parent *models.Place) *models.AccessibilityProfile {
	if child == nil {
		return nil
	}

	effective := &models.AccessibilityProfile{}

	if child.Accessibility != nil {
		ca := child.Accessibility
		effective.ID = ca.ID
		effective.PlaceID = ca.PlaceID
		effective.SourceReports = ca.SourceReports
		effective.UserVerified = ca.UserVerified
		effective.SubmittedAt = ca.SubmittedAt
		effective.UpdatedAt = ca.UpdatedAt
		effective.Entrance = copyEntrance(ca.Entrance)
		effective.Pathways = copyPathways(ca.Pathways)
		effective.Restroom = copyRestroom(ca.Restroom)
		effective.Parking = copyParking(ca.Parking)
		effective.Elevator = copyElevator(ca.Elevator)
	}

	if parent == nil || parent.Accessibility == nil {
		return effective
	}

	pa := parent.Accessibility

	if effective.Entrance == nil && pa.Entrance != nil {
		c := copyEntrance(pa.Entrance)
		c.IsInherited = true
		c.SourceID = parent.ID
		effective.Entrance = c
	}
	if effective.Pathways == nil && pa.Pathways != nil {
		c := copyPathways(pa.Pathways)
		c.IsInherited = true
		c.SourceID = parent.ID
		effective.Pathways = c
	}
	if effective.Restroom == nil && pa.Restroom != nil {
		c := copyRestroom(pa.Restroom)
		c.IsInherited = true
		c.SourceID = parent.ID
		effective.Restroom = c
	}
	if effective.Parking == nil && pa.Parking != nil {
		c := copyParking(pa.Parking)
		c.IsInherited = true
		c.SourceID = parent.ID
		effective.Parking = c
	}
	if effective.Elevator == nil && pa.Elevator != nil {
		c := copyElevator(pa.Elevator)
		c.IsInherited = true
		c.SourceID = parent.ID
		effective.Elevator = c
	}

	return effective
}

func entranceFlags(p *models.EntranceProps) []string {
	var flags []string
	if p.Width != nil && *p.Width == models.LevelNo {
		flags = append(flags, FlagEntranceNarrowWidth)
	}
	// Only flag when IsLevel is explicitly false; nil means unknown, no flag.
	if p.IsLevel != nil && !*p.IsLevel {
		hasRamp := (p.HasFixedRamp != nil && *p.HasFixedRamp) || (p.HasRemovableRamp != nil && *p.HasRemovableRamp)
		if !hasRamp {
			flags = append(flags, FlagEntranceNoLevelRoute)
		}
	}
	return flags
}

func pathwayFlags(p *models.PathwayProps) []string {
	var flags []string
	if p.Width != nil && *p.Width == models.LevelNo {
		flags = append(flags, FlagPathwayNarrowWidth)
	}
	return flags
}

func restroomFlags(p *models.RestroomProps) []string {
	var flags []string
	if p.DoorWidth != nil && *p.DoorWidth == models.LevelNo {
		flags = append(flags, FlagRestroomNarrowDoor)
	}
	if p.TurningRadius != nil && *p.TurningRadius == models.LevelNo {
		flags = append(flags, FlagRestroomSmallTurning)
	}
	if p.HasGrabRails != nil && !*p.HasGrabRails {
		flags = append(flags, FlagRestroomNoGrabRails)
	}
	return flags
}

func parkingFlags(p *models.ParkingProps) []string {
	var flags []string
	if p.HasDisabledSpaces != nil && !*p.HasDisabledSpaces {
		flags = append(flags, FlagParkingNoDisabledSpaces)
	}
	return flags
}

func elevatorFlags(p *models.ElevatorProps) []string {
	var flags []string
	if p.Width != nil && *p.Width == models.LevelNo {
		flags = append(flags, FlagElevatorNarrowWidth)
	}
	if p.Depth != nil && *p.Depth == models.LevelNo {
		flags = append(flags, FlagElevatorShallowDepth)
	}
	if p.DoorWidth != nil && *p.DoorWidth == models.LevelNo {
		flags = append(flags, FlagElevatorNarrowDoor)
	}
	if p.HasBraille != nil && !*p.HasBraille {
		flags = append(flags, FlagElevatorNoBraille)
	}
	if p.HasAudio != nil && !*p.HasAudio {
		flags = append(flags, FlagElevatorNoAudio)
	}
	return flags
}

func copyEntrance(p *models.EntranceProps) *models.EntranceProps {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

func copyPathways(p *models.PathwayProps) *models.PathwayProps {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

func copyRestroom(p *models.RestroomProps) *models.RestroomProps {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

func copyParking(p *models.ParkingProps) *models.ParkingProps {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

func copyElevator(p *models.ElevatorProps) *models.ElevatorProps {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}
