/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

// Package osm contains all OpenStreetMap-specific ingestion logic:
// PBF streaming, POI filtering, and transformation to models.Place.
package osm

import "github.com/InWheelOrg/inwheel-api/pkg/models"

// amenityToCategory maps OSM amenity tag values to our category taxonomy.
var amenityToCategory = map[string]models.Category{
	// Restaurants and food
	"restaurant": models.CategoryRestaurant,
	"fast_food":  models.CategoryRestaurant,
	"food_court": models.CategoryRestaurant,
	"cafe":       models.CategoryCafe,
	"ice_cream":  models.CategoryCafe,
	"biergarten": models.CategoryCafe,
	"bar":        models.CategoryBar,
	"pub":        models.CategoryBar,
	"nightclub":  models.CategoryBar,

	// Healthcare
	"hospital":     models.CategoryHealthcare,
	"clinic":       models.CategoryHealthcare,
	"doctors":      models.CategoryHealthcare,
	"dentist":      models.CategoryHealthcare,
	"pharmacy":     models.CategoryHealthcare,
	"veterinary":   models.CategoryHealthcare,
	"nursing_home": models.CategoryHealthcare,

	// Education
	"school":             models.CategoryEducation,
	"university":         models.CategoryEducation,
	"college":            models.CategoryEducation,
	"kindergarten":       models.CategoryEducation,
	"childcare":          models.CategoryEducation,
	"library":            models.CategoryEducation,
	"language_school":    models.CategoryEducation,
	"music_school":       models.CategoryEducation,
	"driving_school":     models.CategoryEducation,
	"research_institute": models.CategoryEducation,

	// Finance
	"bank":             models.CategoryFinance,
	"atm":              models.CategoryFinance,
	"bureau_de_change": models.CategoryFinance,
	"money_transfer":   models.CategoryFinance,

	// Entertainment
	"cinema":           models.CategoryEntertainment,
	"theatre":          models.CategoryEntertainment,
	"arts_centre":      models.CategoryEntertainment,
	"casino":           models.CategoryEntertainment,
	"community_centre": models.CategoryEntertainment,
	"events_venue":     models.CategoryEntertainment,
	"social_centre":    models.CategoryEntertainment,

	// Government
	"townhall":     models.CategoryGovernment,
	"courthouse":   models.CategoryGovernment,
	"embassy":      models.CategoryGovernment,
	"police":       models.CategoryGovernment,
	"fire_station": models.CategoryGovernment,
	"post_office":  models.CategoryGovernment,

	// Transport (amenity-tagged)
	"bus_station":    models.CategoryTransport,
	"ferry_terminal": models.CategoryTransport,
	"taxi":           models.CategoryTransport,
	"car_rental":     models.CategoryTransport,
	"bicycle_rental": models.CategoryTransport,
	"car_sharing":    models.CategoryTransport,

	// Social
	"social_facility": models.CategorySocial,
	"shelter":         models.CategorySocial,

	// Worship
	"place_of_worship": models.CategoryWorship,

	// Other (everyday utility POIs)
	"toilets":       models.CategoryToilet,
	"marketplace":   models.CategoryOther,
	"internet_cafe": models.CategoryOther,
}

// publicTransportAllowlist enumerates public_transport tag values worth importing as transport POIs.
var publicTransportAllowlist = map[string]struct{}{
	"station":   {},
	"stop_area": {},
}

// buildingQualifyingKeys are the tag keys that, when present alongside building=*, qualify a building as a POI.
var buildingQualifyingKeys = map[string]struct{}{
	"amenity":          {},
	"shop":             {},
	"tourism":          {},
	"leisure":          {},
	"office":           {},
	"healthcare":       {},
	"public_transport": {},
}

// Evaluate decides whether an OSM element's tags qualify it as a POI we want to ingest.
// Returns the matched category and true on inclusion, or empty category and false on exclusion.
//
// Allowlist-based: unknown tags are excluded by default. Order matters because some elements
// carry multiple qualifying tags (e.g. amenity=hospital + building=yes) — amenity wins.
func Evaluate(tags map[string]string) (models.Category, bool) {
	if cat, ok := amenityToCategory[tags["amenity"]]; ok {
		return cat, true
	}

	if pt, ok := tags["public_transport"]; ok {
		if _, allowed := publicTransportAllowlist[pt]; allowed {
			return models.CategoryTransport, true
		}
	}

	if shop := tags["shop"]; shop != "" {
		return models.CategoryShop, true
	}

	if tags["building"] != "" {
		for key := range buildingQualifyingKeys {
			if tags[key] != "" {
				// A building qualified by some other tag; recurse on that key by re-evaluating.
				// In practice the amenity / shop / public_transport branches above already caught
				// most cases. This branch handles building=* + an unmapped qualifying tag — return
				// CategoryOther so we don't lose buildings that look like POIs.
				return models.CategoryOther, true
			}
		}
	}

	return "", false
}
