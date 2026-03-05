/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package models

// A11yStatus defines the overall accessibility state of a place.
type A11yStatus string

const (
	// StatusAccessible means the place is fully accessible.
	StatusAccessible A11yStatus = "accessible"
	// StatusLimited means the place is partially accessible (e.g., requires assistance).
	StatusLimited A11yStatus = "limited"
	// StatusInaccessible means the place is not accessible.
	StatusInaccessible A11yStatus = "inaccessible"
	// StatusUnknown means accessibility information is not available.
	StatusUnknown A11yStatus = "unknown"
)

// AccessibilityProfile summarizes the accessibility of a place.
type AccessibilityProfile struct {
	// OverallStatus is the summary rating of accessibility.
	OverallStatus A11yStatus `json:"overall_status"`
	// Components are the individual accessibility features (entrance, etc).
	Components []A11yComponent `json:"components,omitzero"`
	// Audit contains the findings of the automated accessibility audit.
	Audit *AuditResult `json:"audit,omitzero"`
}

// AuditResult contains the findings of the automated accessibility audit.
type AuditResult struct {
	// HasConflict is true if the AI detected logical contradictions.
	HasConflict bool `json:"has_conflict"`
	// Reasoning explains why a conflict was flagged (e.g., "Accessible but has steps").
	Reasoning string `json:"reasoning"`
	// Confidence is the AI's certainty score (0.0 to 1.0).
	Confidence float64 `json:"confidence"`
	// LastAudit is the ISO8601 timestamp of the last audit pass.
	LastAudit string `json:"last_audit"`
}

// A11yComponentType identifies the kind of accessibility feature.
type A11yComponentType string

const (
	ComponentEntrance A11yComponentType = "entrance"
	ComponentRestroom A11yComponentType = "restroom"
	ComponentParking  A11yComponentType = "parking"
	ComponentElevator A11yComponentType = "elevator"
	ComponentOther    A11yComponentType = "other"
)

// A11yComponent represents a modular accessibility feature.
type A11yComponent struct {
	// Type is the kind of component.
	Type A11yComponentType `json:"type"`
	// IsInherited is true if the component data is inherited from a parent place.
	IsInherited bool `json:"is_inherited"`
	// SourceID is the ID of the Place that owns this specific data.
	SourceID string `json:"source_id"`
	// OverallStatus is the summary rating of this specific component.
	OverallStatus A11yStatus `json:"overall_status"`
	// Entrance contains properties for an entrance component.
	Entrance *EntranceProperties `json:"entrance,omitzero"`
	// Restroom contains properties for a restroom component.
	Restroom *RestroomProperties `json:"restroom,omitzero"`
	// Parking contains properties for a parking component.
	Parking *ParkingProperties `json:"parking,omitzero"`
	// Elevator contains properties for an elevator component.
	Elevator *ElevatorProperties `json:"elevator,omitzero"`
	// Metadata contains additional un-modeled tags or source-specific data.
	Metadata map[string]any `json:"metadata,omitzero"`
}

// EntranceProperties contains technical details about an entrance.
type EntranceProperties struct {
	// Width is the clear opening width in meters.
	Width *float64 `json:"width,omitzero"`
	// HasRamp indicates if a ramp is present.
	HasRamp *bool `json:"has_ramp,omitzero"`
	// IsAutomatic indicates if the door is automatic.
	IsAutomatic *bool `json:"is_automatic,omitzero"`
	// HasStep indicates if there is a step at the entrance.
	HasStep *bool `json:"has_step,omitzero"`
	// StepHeight is the height of the step in meters.
	StepHeight *float64 `json:"step_height,omitzero"`
}

// RestroomProperties contains details about a restroom feature.
type RestroomProperties struct {
	// WheelchairAccessible indicates if the restroom is accessible to wheelchairs.
	WheelchairAccessible *bool `json:"wheelchair_accessible,omitzero"`
	// GrabRails indicates if grab rails are installed.
	GrabRails *bool `json:"grab_rails,omitzero"`
	// ChangingTable indicates if a diaper changing table is available.
	ChangingTable *bool `json:"changing_table,omitzero"`
	// DoorWidth is the width of the restroom door in meters.
	DoorWidth *float64 `json:"door_width,omitzero"`
}

// ParkingProperties contains details about disabled parking.
type ParkingProperties struct {
	// HasDisabledSpaces indicates if there are dedicated disabled parking spots.
	HasDisabledSpaces *bool `json:"has_disabled_spaces,omitzero"`
	// Count is the number of disabled parking spaces available.
	Count *int `json:"count,omitzero"`
}

// ElevatorProperties contains technical details about an elevator.
type ElevatorProperties struct {
	// Width is the elevator cabin width in meters.
	Width *float64 `json:"width,omitzero"`
	// Depth is the elevator cabin depth in meters.
	Depth *float64 `json:"depth,omitzero"`
	// Braille indicates if there are braille labels on the buttons.
	Braille *bool `json:"braille,omitzero"`
	// Audio indicates if there are audio announcements.
	Audio *bool `json:"audio,omitzero"`
}
