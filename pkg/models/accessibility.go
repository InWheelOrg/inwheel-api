/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// AccessibilityProfile stores factual accessibility data for a place.
// Component fields hold measured facts; SourceReports carries raw opinions from
// external sources (e.g. OSM wheelchair=yes). No server-side accessibility
// judgment is made; clients apply their own logic per user need.
type AccessibilityProfile struct {
	ID            string         `json:"id,omitempty" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	PlaceID       string         `json:"place_id,omitempty" gorm:"uniqueIndex;type:uuid"`
	SourceReports SourceReports  `json:"source_reports,omitempty" gorm:"type:jsonb"`
	Entrance      *EntranceProps `json:"entrance,omitempty" gorm:"type:jsonb"`
	Pathways      *PathwayProps  `json:"pathways,omitempty" gorm:"type:jsonb"`
	Restroom      *RestroomProps `json:"restroom,omitempty" gorm:"type:jsonb"`
	Parking       *ParkingProps  `json:"parking,omitempty" gorm:"type:jsonb"`
	Elevator      *ElevatorProps `json:"elevator,omitempty" gorm:"type:jsonb"`
	UserVerified  bool           `json:"user_verified,omitempty"`
	SubmittedBy   *string        `json:"-" gorm:"type:uuid"`
	SubmittedAt   *time.Time     `json:"submitted_at,omitempty"`
	UpdatedAt     time.Time      `json:"updated_at,omitzero"`
}

// SourceReport is a raw opinion about accessibility from a named external source.
// Records what a source said without interpreting it. Clients decide trust level.
type SourceReport struct {
	Source     string    `json:"source"`      // e.g. "osm", "wheelmap", "user"
	Value      string    `json:"value"`       // raw: "yes", "limited", "no"
	RecordedAt time.Time `json:"recorded_at"`
}

type SourceReports []SourceReport

func (s *SourceReports) Scan(value interface{}) error { return scanJSONB(s, value) }
func (s SourceReports) Value() (driver.Value, error)  { return marshalJSONB(s) }

type DoorType string

const (
	DoorAutomatic DoorType = "automatic"
	DoorManual    DoorType = "manual"
	DoorRevolving DoorType = "revolving"
	DoorNone      DoorType = "none"
)

type SurfaceType string

const (
	SurfaceAsphalt      SurfaceType = "asphalt"
	SurfacePavingStones SurfaceType = "paving_stones"
	SurfaceCobblestone  SurfaceType = "cobblestone"
	SurfaceGravel       SurfaceType = "gravel"
	SurfaceConcrete     SurfaceType = "concrete"
	SurfaceWood         SurfaceType = "wood"
	SurfaceCarpet       SurfaceType = "carpet"
	SurfaceTiles        SurfaceType = "tiles"
)

type DoorProps struct {
	Type  DoorType `json:"type,omitempty"`
	Width *float64 `json:"width,omitempty"` // metres, clear opening
}

// EntranceProps. All dimensions in metres. AuditFlags are computed on write.
// IsInherited and SourceID are set by ComputeEffectiveProfile at read time, never stored.
type EntranceProps struct {
	IsLevel          *bool      `json:"is_level,omitempty"`
	HasFixedRamp     *bool      `json:"has_fixed_ramp,omitempty"`
	HasRemovableRamp *bool      `json:"has_removable_ramp,omitempty"`
	SlopePercent     *float64   `json:"slope_percent,omitempty"`
	Width            *float64   `json:"width,omitempty"`
	Door             *DoorProps `json:"door,omitempty"`
	HasIntercom      *bool      `json:"has_intercom,omitempty"`
	AuditFlags       []string   `json:"audit_flags,omitempty"`
	IsInherited      bool       `json:"is_inherited,omitempty"`
	SourceID         string     `json:"source_id,omitempty"`
}

func (p *EntranceProps) Scan(value interface{}) error { return scanJSONB(p, value) }
func (p EntranceProps) Value() (driver.Value, error)  { return marshalJSONB(p) }

type PathwayProps struct {
	Width           *float64    `json:"width,omitempty"` // narrowest passage in metres
	Surface         SurfaceType `json:"surface,omitempty"`
	IsKerbstoneFree *bool       `json:"is_kerbstone_free,omitempty"`
	HasSteps        *bool       `json:"has_steps,omitempty"`
	AuditFlags      []string    `json:"audit_flags,omitempty"`
	IsInherited     bool        `json:"is_inherited,omitempty"`
	SourceID        string      `json:"source_id,omitempty"`
}

func (p *PathwayProps) Scan(value interface{}) error { return scanJSONB(p, value) }
func (p PathwayProps) Value() (driver.Value, error)  { return marshalJSONB(p) }

// RestroomProps. All dimensions in metres.
type RestroomProps struct {
	IsAccessible     *bool    `json:"is_accessible,omitempty"`
	DoorWidth        *float64 `json:"door_width,omitempty"`
	TurningRadius    *float64 `json:"turning_radius,omitempty"`
	HasGrabRails     *bool    `json:"has_grab_rails,omitempty"`
	HasRollInShower  *bool    `json:"has_roll_in_shower,omitempty"`
	ToiletSeatHeight *float64 `json:"toilet_seat_height,omitempty"`
	HasEmergencyPull *bool    `json:"has_emergency_pull,omitempty"`
	HasChangingTable *bool    `json:"has_changing_table,omitempty"`
	AuditFlags       []string `json:"audit_flags,omitempty"`
	IsInherited      bool     `json:"is_inherited,omitempty"`
	SourceID         string   `json:"source_id,omitempty"`
}

func (p *RestroomProps) Scan(value interface{}) error { return scanJSONB(p, value) }
func (p RestroomProps) Value() (driver.Value, error)  { return marshalJSONB(p) }

// ParkingProps. DistanceToEntrance and Width in metres.
type ParkingProps struct {
	HasDisabledSpaces   *bool    `json:"has_disabled_spaces,omitempty"`
	Count               *int     `json:"count,omitempty"`
	DistanceToEntrance  *float64 `json:"distance_to_entrance,omitempty"`
	Width               *float64 `json:"width,omitempty"`
	HasDedicatedSignage *bool    `json:"has_dedicated_signage,omitempty"`
	AuditFlags          []string `json:"audit_flags,omitempty"`
	IsInherited         bool     `json:"is_inherited,omitempty"`
	SourceID            string   `json:"source_id,omitempty"`
}

func (p *ParkingProps) Scan(value interface{}) error { return scanJSONB(p, value) }
func (p ParkingProps) Value() (driver.Value, error)  { return marshalJSONB(p) }

// ElevatorProps. Width, Depth, and DoorWidth in metres.
type ElevatorProps struct {
	Width       *float64 `json:"width,omitempty"`
	Depth       *float64 `json:"depth,omitempty"`
	DoorWidth   *float64 `json:"door_width,omitempty"`
	HasBraille  *bool    `json:"has_braille,omitempty"`
	HasAudio    *bool    `json:"has_audio,omitempty"`
	AuditFlags  []string `json:"audit_flags,omitempty"`
	IsInherited bool     `json:"is_inherited,omitempty"`
	SourceID    string   `json:"source_id,omitempty"`
}

func (p *ElevatorProps) Scan(value interface{}) error { return scanJSONB(p, value) }
func (p ElevatorProps) Value() (driver.Value, error)  { return marshalJSONB(p) }

func scanJSONB(dest any, value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, dest)
}

func marshalJSONB(v any) (driver.Value, error) {
	return json.Marshal(v)
}
