/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package geo

import (
	"github.com/InWheelOrg/inwheel-api/pkg/models"
	"gorm.io/gorm"
)

// FindNearbyPlaces retrieves places within a given radius (in meters) from a point.
func FindNearbyPlaces(db *gorm.DB, lng, lat float64, radius float64) ([]models.Place, error) {
	var places []models.Place

	err := db.Where("ST_DWithin(geography(ST_Point(lng, lat)), geography(ST_Point(?, ?)), ?)",
		lng, lat, radius).Find(&places).Error

	return places, err
}

// FindPlacesInBoundingBox retrieves places within a rectangular bounding box.
func FindPlacesInBoundingBox(db *gorm.DB, minLng, minLat, maxLng, maxLat float64) ([]models.Place, error) {
	var places []models.Place

	err := db.Where("geography(ST_Point(lng, lat)) && ST_MakeEnvelope(?, ?, ?, ?, 4326)::geography",
		minLng, minLat, maxLng, maxLat).Find(&places).Error

	return places, err
}
