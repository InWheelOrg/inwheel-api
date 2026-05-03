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

// Rank defines the zoom-level hierarchy of a physical location.
type Rank int

const (
	// RankLandmark represents a major city landmark or transit hub (high priority).
	RankLandmark Rank = 1
	// RankEstablishment represents a standard commercial or public building.
	RankEstablishment Rank = 2
	// RankFeature represents a minor feature like a bench, entrance, or toilet.
	RankFeature Rank = 3
)

// PlaceStatus represents the lifecycle state of a place.
type PlaceStatus string

const (
	// PlaceStatusActive means the place exists in OSM and is fully operational.
	PlaceStatusActive PlaceStatus = "active"
	// PlaceStatusClosed means the place has been reported as permanently closed.
	PlaceStatusClosed PlaceStatus = "closed"
	// PlaceStatusOSMRemoved means OSM removed the place but user accessibility data exists.
	PlaceStatusOSMRemoved PlaceStatus = "osm_removed"
)

// Category represents the classification of a place.
type Category string

const (
	CategoryMall         Category = "mall"
	CategoryAirport      Category = "airport"
	CategoryTrainStation Category = "train_station"
	CategoryRestaurant   Category = "restaurant"
	CategoryCafe         Category = "cafe"
	CategoryShop         Category = "shop"
	CategoryToilet       Category = "toilet"
	CategoryParking      Category = "parking"
	CategoryEntrance     Category = "entrance"
	CategoryOther        Category = "other"
)

// OSMType represents the OpenStreetMap data type.
type OSMType string

const (
	// OSMNode represents a single point.
	OSMNode OSMType = "node"
	// OSMWay represents a polyline or polygon.
	OSMWay OSMType = "way"
	// OSMRelation represents a collection of nodes, ways, or other relations.
	OSMRelation OSMType = "relation"
)

// Place is the identity layer model, representing a physical location.
type Place struct {
	// ID is the unique internal identifier.
	ID string `json:"id,omitzero" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	// OSMID is the original OpenStreetMap ID.
	OSMID int64 `json:"osm_id,omitzero"`
	// OSMType is the original OpenStreetMap data type.
	OSMType OSMType `json:"osm_type,omitzero"`
	// Name is the human-readable name of the place.
	Name string `json:"name"`
	// Lng is the longitude of the place.
	Lng float64 `json:"lng"`
	// Lat is the latitude of the place.
	Lat float64 `json:"lat"`
	// Geometry contains the spatial representation of the place.
	Geometry *Geometry `json:"geometry,omitzero" gorm:"type:jsonb"`
	// Category is the classification of the place.
	Category Category `json:"category"`
	// Rank is the zoom-level priority of the place.
	Rank Rank `json:"rank,omitzero"`
	// ParentID is the identifier of the containing place (e.g., a shop's mall).
	ParentID *string `json:"parent_id,omitzero"`
	// Accessibility contains the accessibility profile of the place.
	Accessibility *AccessibilityProfile `json:"accessibility,omitzero" gorm:"foreignKey:PlaceID"`
	// Tags contain additional key-value data from the source.
	Tags PlaceTags `json:"tags,omitzero" gorm:"type:jsonb"`
	// ExternalIDs stores identifiers from external sources keyed by source name (e.g. {"osm": "node/123456"}).
	ExternalIDs ExternalIDs `json:"external_ids,omitzero" gorm:"type:jsonb"`
	// Status is the lifecycle state of the place.
	Status PlaceStatus `json:"status,omitzero" gorm:"type:place_status;default:active"`
	// Source indicates where the data originated (e.g., "osm").
	Source string `json:"source,omitzero"`
	// SubmittedBy is the ID of the API key that last wrote this record. Internal only.
	SubmittedBy *string `json:"-" gorm:"type:uuid"`
	// SubmittedAt is the timestamp of the last submission.
	SubmittedAt *time.Time `json:"submitted_at,omitzero"`
	// CreatedAt is the timestamp when the place was created.
	CreatedAt time.Time `json:"created_at,omitzero"`
	// UpdatedAt is the timestamp when the place was last updated.
	UpdatedAt time.Time `json:"updated_at,omitzero"`
}

// Geometry represents a GeoJSON-compatible geometry object.
type Geometry struct {
	// Type is the GeoJSON geometry type (Point, Polygon, etc.).
	Type string `json:"type"`
	// Coordinates contains the GeoJSON coordinates (point/polygon/etc).
	Coordinates any `json:"coordinates"`
}

// PlaceTags is a custom type, so we can implement SQL scanning.
type PlaceTags map[string]string

// Scan tells the SQL driver how to read the JSONB bytes into the Geometry struct.
func (g *Geometry) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed for Geometry")
	}
	return json.Unmarshal(bytes, g)
}

// Value tells the SQL driver how to write the Geometry to the database as JSONB.
func (g *Geometry) Value() (driver.Value, error) {
	if g == nil {
		return nil, nil
	}
	return json.Marshal(g)
}

// Scan tells the SQL driver how to read the JSONB bytes into the map.
func (t *PlaceTags) Scan(value interface{}) error {
	if value == nil {
		*t = make(PlaceTags)
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed for PlaceTags")
	}
	return json.Unmarshal(bytes, t)
}

// Value tells the SQL driver how to write the map to the database as JSONB.
func (t PlaceTags) Value() (driver.Value, error) {
	if t == nil {
		return json.Marshal(make(PlaceTags))
	}
	return json.Marshal(t)
}

// ExternalIDs maps source names to their external identifiers (e.g. {"osm": "node/123456"}).
type ExternalIDs map[string]string

// Scan tells the SQL driver how to read the JSONB bytes into the map.
func (e *ExternalIDs) Scan(value interface{}) error {
	if value == nil {
		*e = make(ExternalIDs)
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed for ExternalIDs")
	}
	return json.Unmarshal(bytes, e)
}

// Value tells the SQL driver how to write the map to the database as JSONB.
func (e ExternalIDs) Value() (driver.Value, error) {
	if e == nil {
		return nil, nil
	}
	return json.Marshal(e)
}
