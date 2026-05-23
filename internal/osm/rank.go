/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package osm

import "github.com/InWheelOrg/inwheel-api/pkg/models"

// landmarkTransportAmenity identifies major transport hubs that should rank as landmarks.
var landmarkTransportAmenity = map[string]struct{}{
	"bus_station":    {},
	"ferry_terminal": {},
}

// landmarkTransportPublic identifies major public_transport values that should rank as landmarks.
var landmarkTransportPublic = map[string]struct{}{
	"station": {},
}

// featureAmenity identifies amenity values that are minor utilities rather than primary establishments.
var featureAmenity = map[string]struct{}{
	"toilets": {},
	"atm":     {},
	"shelter": {},
}

// DeriveRank picks a zoom-level priority for a place based on its category and OSM tags.
// Returns RankLandmark for major hubs, RankFeature for minor utilities, RankEstablishment otherwise.
func DeriveRank(category models.Category, tags map[string]string) models.Rank {
	amenity := tags["amenity"]
	publicTransport := tags["public_transport"]

	switch category {
	case models.CategoryHealthcare:
		if amenity == "hospital" {
			return models.RankLandmark
		}
	case models.CategoryEducation:
		if amenity == "university" {
			return models.RankLandmark
		}
	case models.CategoryTransport:
		if _, ok := landmarkTransportAmenity[amenity]; ok {
			return models.RankLandmark
		}
		if _, ok := landmarkTransportPublic[publicTransport]; ok {
			return models.RankLandmark
		}
	case models.CategoryToilet:
		return models.RankFeature
	case models.CategoryFinance, models.CategorySocial:
		if _, ok := featureAmenity[amenity]; ok {
			return models.RankFeature
		}
	}

	return models.RankEstablishment
}
