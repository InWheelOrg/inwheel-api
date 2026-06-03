/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package osm

import (
	"fmt"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// TransformNode converts a filtered OSM node into a models.Place ready for upsert.
// The category must come from a prior call to Evaluate.
func TransformNode(osmID int64, lat, lng float64, tags map[string]string, category models.Category) (*models.Place, error) {
	if category == "" {
		return nil, fmt.Errorf("transform: category is empty for node %d", osmID)
	}

	placeTags := make(models.PlaceTags, len(tags))
	for k, v := range tags {
		placeTags[k] = v
	}

	return &models.Place{
		OSMID:    osmID,
		OSMType:  models.OSMNode,
		Name:     tags["name"],
		Lat:      lat,
		Lng:      lng,
		Category: category,
		Rank:     DeriveRank(category, tags),
		Tags:     placeTags,
		ExternalIDs: models.ExternalIDs{
			"osm": fmt.Sprintf("node/%d", osmID),
		},
		Source: "osm",
		Status: models.PlaceStatusActive,
	}, nil
}
