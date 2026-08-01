/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package osm

import (
	"fmt"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// TransformOSMNode converts a filtered OSM node into a models.Place and an optional
// AccessibilityProfile. The category must come from a prior call to Evaluate.
func TransformOSMNode(node Node, category models.Category) (*models.Place, *models.AccessibilityProfile, error) {
	return transform(node.ID, node.Lat, node.Lng, node.Tags, category, models.OSMNode)
}

// TransformOSMWay converts a filtered OSM way into a models.Place and an optional
// AccessibilityProfile. The category must come from a prior call to Evaluate.
func TransformOSMWay(way Way, category models.Category) (*models.Place, *models.AccessibilityProfile, error) {
	return transform(way.ID, way.Lat, way.Lng, way.Tags, category, models.OSMWay)
}

// transform converts a filtered OSM node or way into a models.Place and an optional
// AccessibilityProfile. Profile is nil when no accessibility tags are present.
func transform(osmID int64, lat, lng float64, tags map[string]string, category models.Category, osmType models.OSMType) (*models.Place, *models.AccessibilityProfile, error) {
	if category == "" {
		return nil, nil, fmt.Errorf("transform: category is empty for %s %d", osmType, osmID)
	}

	placeTags := make(models.PlaceTags, len(tags))
	for k, v := range tags {
		placeTags[k] = v
	}

	place := &models.Place{
		OSMID:    osmID,
		OSMType:  osmType,
		Name:     tags["name"],
		Lat:      lat,
		Lng:      lng,
		Category: category,
		Rank:     DeriveRank(category, tags),
		Tags:     placeTags,
		ExternalIDs: models.ExternalIDs{
			"osm": models.ExternalRef{
				ID:         fmt.Sprintf("%s/%d", osmType, osmID),
				Confidence: 1.0,
			},
		},
		Source: "osm",
		Status: models.PlaceStatusActive,
	}
	return place, mapTagsToProfile(tags), nil
}
