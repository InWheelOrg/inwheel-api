/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package models

import "time"

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

// Category represents the classification of a place.
type Category string

const (
	CategoryMall         Category = "mall"
	CategoryAirport               = "airport"
	CategoryTrainStation          = "train_station"
	CategoryRestaurant            = "restaurant"
	CategoryCafe                  = "cafe"
	CategoryShop                  = "shop"
	CategoryToilet                = "toilet"
	CategoryParking               = "parking"
	CategoryEntrance              = "entrance"
	CategoryOther                 = "other"
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

// Geometry represents a GeoJSON-compatible geometry object.
type Geometry struct {
	// Type is the GeoJSON geometry type (Point, Polygon, etc.).
	Type string `json:"type"`
	// Coordinates contains the GeoJSON coordinates (point/polygon/etc).
	Coordinates any `json:"coordinates"`
}

// Place is the identity layer model, representing a physical location.
type Place struct {
	// ID is the unique internal identifier.
	ID string `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	// OSMID is the original OpenStreetMap ID.
	OSMID int64 `json:"osm_id"`
	// OSMType is the original OpenStreetMap data type.
	OSMType OSMType `json:"osm_type"`
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
	Rank Rank `json:"rank"`
	// ParentID is the identifier of the containing place (e.g., a shop's mall).
	ParentID *string `json:"parent_id,omitzero"`
	// Accessibility contains the accessibility profile of the place.
	Accessibility *AccessibilityProfile `json:"accessibility,omitzero" gorm:"type:jsonb"`
	// Tags contain additional key-value data from the source.
	Tags map[string]string `json:"tags,omitzero" gorm:"type:jsonb"`
	// Source indicates where the data originated (e.g., "osm").
	Source string `json:"source"`
	// NeedsAudit is an internal flag used to queue the place for an accessibility audit.
	NeedsAudit bool `json:"-" gorm:"index"`
	// CreatedAt is the timestamp when the place was created.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is the timestamp when the place was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}
