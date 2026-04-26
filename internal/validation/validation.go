/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

// Package validation enforces structural and bounds constraints on incoming API
// requests. Runs before the a11y engine: malformed input → 400 with a field-level
// error list; the engine handles business consistency (audit flags, conflicts → 422).
package validation

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/InWheelOrg/inwheel-server/pkg/models"
)

// FieldError describes a single rejected field. Matches the JSON shape returned
// in 400 validation responses.
type FieldError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

const (
	maxNameLength      = 256
	maxSourceLength    = 64
	maxTagEntries      = 50
	maxTagKeyLength    = 64
	maxTagValueLength  = 256
	maxMetadataEntries = 50
	maxMetadataBytes   = 4 * 1024

	maxDimensionMeters    = 10.0
	maxStepHeightMeters   = 1.0
	maxParkingCount       = 10000
	maxRadiusMeters       = 50000.0
	worldLatMin           = -90.0
	worldLatMax           = 90.0
	worldLngMin           = -180.0
	worldLngMax           = 180.0
)

var (
	uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

	validCategories = map[models.Category]bool{
		models.CategoryMall: true, models.CategoryAirport: true,
		models.CategoryTrainStation: true, models.CategoryRestaurant: true,
		models.CategoryCafe: true, models.CategoryShop: true,
		models.CategoryToilet: true, models.CategoryParking: true,
		models.CategoryEntrance: true, models.CategoryOther: true,
	}
	validOSMTypes = map[models.OSMType]bool{
		models.OSMNode: true, models.OSMWay: true, models.OSMRelation: true,
	}
	validRanks = map[models.Rank]bool{
		models.RankLandmark: true, models.RankEstablishment: true, models.RankFeature: true,
	}
	validStatuses = map[models.A11yStatus]bool{
		models.StatusAccessible: true, models.StatusLimited: true,
		models.StatusInaccessible: true, models.StatusUnknown: true,
	}
	validComponentTypes = map[models.A11yComponentType]bool{
		models.ComponentEntrance: true, models.ComponentRestroom: true,
		models.ComponentParking: true, models.ComponentElevator: true,
		models.ComponentOther: true,
	}
)

// Place validates a *models.Place for structural correctness. Returns nil if valid.
func Place(p *models.Place) []FieldError {
	if p == nil {
		return []FieldError{{Field: "body", Reason: "place is required"}}
	}

	var errs []FieldError

	if strings.TrimSpace(p.Name) == "" {
		errs = append(errs, FieldError{Field: "name", Reason: "is required"})
	} else if len(p.Name) > maxNameLength {
		errs = append(errs, FieldError{Field: "name", Reason: fmt.Sprintf("must be ≤ %d characters", maxNameLength)})
	}

	errs = append(errs, validateLat("lat", p.Lat)...)
	errs = append(errs, validateLng("lng", p.Lng)...)

	if p.Category == "" {
		errs = append(errs, FieldError{Field: "category", Reason: "is required"})
	} else if !validCategories[p.Category] {
		errs = append(errs, FieldError{Field: "category", Reason: fmt.Sprintf("%q is not a valid category", string(p.Category))})
	}

	if p.Rank != 0 && !validRanks[p.Rank] {
		errs = append(errs, FieldError{Field: "rank", Reason: "must be 1, 2, or 3"})
	}

	if p.OSMID < 0 {
		errs = append(errs, FieldError{Field: "osm_id", Reason: "must be positive"})
	}

	if p.OSMType != "" && !validOSMTypes[p.OSMType] {
		errs = append(errs, FieldError{Field: "osm_type", Reason: fmt.Sprintf("%q is not a valid OSM type", string(p.OSMType))})
	}

	if len(p.Source) > maxSourceLength {
		errs = append(errs, FieldError{Field: "source", Reason: fmt.Sprintf("must be ≤ %d characters", maxSourceLength)})
	}

	if p.ParentID != nil && !uuidRegex.MatchString(*p.ParentID) {
		errs = append(errs, FieldError{Field: "parent_id", Reason: "must be a valid UUID"})
	}

	errs = append(errs, validateTags(p.Tags)...)

	if p.Accessibility != nil {
		for _, e := range AccessibilityProfile(p.Accessibility) {
			e.Field = "accessibility." + e.Field
			errs = append(errs, e)
		}
	}

	return errs
}

// AccessibilityProfile validates a *models.AccessibilityProfile (used standalone for
// PATCH /places/{id}/accessibility, and recursively from Place).
func AccessibilityProfile(prof *models.AccessibilityProfile) []FieldError {
	if prof == nil {
		return nil
	}
	var errs []FieldError

	if prof.OverallStatus == "" {
		errs = append(errs, FieldError{Field: "overall_status", Reason: "is required"})
	} else if !validStatuses[prof.OverallStatus] {
		errs = append(errs, FieldError{Field: "overall_status", Reason: fmt.Sprintf("%q is not a valid status", string(prof.OverallStatus))})
	}

	for i, comp := range prof.Components {
		prefix := fmt.Sprintf("components[%d]", i)
		for _, e := range validateComponent(comp) {
			e.Field = prefix + "." + e.Field
			errs = append(errs, e)
		}
	}

	return errs
}

// PlaceID validates a path-param ID is a well-formed UUID.
func PlaceID(id string) []FieldError {
	if !uuidRegex.MatchString(id) {
		return []FieldError{{Field: "id", Reason: "must be a valid UUID"}}
	}
	return nil
}

// PlacesQuery validates the query string of GET /places.
func PlacesQuery(q url.Values) []FieldError {
	circular := []string{q.Get("lng"), q.Get("lat"), q.Get("radius")}
	bbox := []string{q.Get("min_lng"), q.Get("min_lat"), q.Get("max_lng"), q.Get("max_lat")}

	circularPresent := nonEmptyCount(circular)
	bboxPresent := nonEmptyCount(bbox)

	if circularPresent != 0 && circularPresent != len(circular) {
		return []FieldError{{Field: "query", Reason: "lng, lat, and radius must all be provided together"}}
	}
	if bboxPresent != 0 && bboxPresent != len(bbox) {
		return []FieldError{{Field: "query", Reason: "min_lng, min_lat, max_lng, and max_lat must all be provided together"}}
	}
	if circularPresent > 0 && bboxPresent > 0 {
		return []FieldError{{Field: "query", Reason: "circular and bounding-box parameters are mutually exclusive"}}
	}

	var errs []FieldError

	if circularPresent == len(circular) {
		lng, e1 := parseFloat("lng", q.Get("lng"))
		lat, e2 := parseFloat("lat", q.Get("lat"))
		radius, e3 := parseFloat("radius", q.Get("radius"))
		errs = appendIfErr(errs, e1, e2, e3)
		if e1 == nil {
			errs = append(errs, validateLng("lng", lng)...)
		}
		if e2 == nil {
			errs = append(errs, validateLat("lat", lat)...)
		}
		if e3 == nil {
			if radius <= 0 || radius > maxRadiusMeters {
				errs = append(errs, FieldError{Field: "radius", Reason: fmt.Sprintf("must be between 0 (exclusive) and %g", maxRadiusMeters)})
			}
		}
	}

	if bboxPresent == len(bbox) {
		minLng, e1 := parseFloat("min_lng", q.Get("min_lng"))
		minLat, e2 := parseFloat("min_lat", q.Get("min_lat"))
		maxLng, e3 := parseFloat("max_lng", q.Get("max_lng"))
		maxLat, e4 := parseFloat("max_lat", q.Get("max_lat"))
		errs = appendIfErr(errs, e1, e2, e3, e4)
		if e1 == nil {
			errs = append(errs, validateLng("min_lng", minLng)...)
		}
		if e3 == nil {
			errs = append(errs, validateLng("max_lng", maxLng)...)
		}
		if e2 == nil {
			errs = append(errs, validateLat("min_lat", minLat)...)
		}
		if e4 == nil {
			errs = append(errs, validateLat("max_lat", maxLat)...)
		}
		if e1 == nil && e3 == nil && minLng >= maxLng {
			errs = append(errs, FieldError{Field: "min_lng", Reason: "must be less than max_lng"})
		}
		if e2 == nil && e4 == nil && minLat >= maxLat {
			errs = append(errs, FieldError{Field: "min_lat", Reason: "must be less than max_lat"})
		}
	}

	return errs
}

func validateComponent(comp models.A11yComponent) []FieldError {
	var errs []FieldError

	if comp.Type == "" {
		errs = append(errs, FieldError{Field: "type", Reason: "is required"})
	} else if !validComponentTypes[comp.Type] {
		errs = append(errs, FieldError{Field: "type", Reason: fmt.Sprintf("%q is not a valid component type", string(comp.Type))})
	}

	if comp.OverallStatus != "" && !validStatuses[comp.OverallStatus] {
		errs = append(errs, FieldError{Field: "overall_status", Reason: fmt.Sprintf("%q is not a valid status", string(comp.OverallStatus))})
	}

	if comp.Entrance != nil {
		errs = append(errs, validateBoundedFloat("entrance.width", comp.Entrance.Width, 0, maxDimensionMeters)...)
		errs = append(errs, validateBoundedFloat("entrance.step_height", comp.Entrance.StepHeight, 0, maxStepHeightMeters)...)
	}
	if comp.Restroom != nil {
		errs = append(errs, validateBoundedFloat("restroom.door_width", comp.Restroom.DoorWidth, 0, maxDimensionMeters)...)
	}
	if comp.Elevator != nil {
		errs = append(errs, validateBoundedFloat("elevator.width", comp.Elevator.Width, 0, maxDimensionMeters)...)
		errs = append(errs, validateBoundedFloat("elevator.depth", comp.Elevator.Depth, 0, maxDimensionMeters)...)
	}
	if comp.Parking != nil && comp.Parking.Count != nil {
		c := *comp.Parking.Count
		if c < 0 || c > maxParkingCount {
			errs = append(errs, FieldError{Field: "parking.count", Reason: fmt.Sprintf("must be between 0 and %d", maxParkingCount)})
		}
	}

	errs = append(errs, validateMetadata(comp.Metadata)...)

	return errs
}

func validateBoundedFloat(field string, v *float64, min, max float64) []FieldError {
	if v == nil {
		return nil
	}
	if math.IsNaN(*v) || math.IsInf(*v, 0) {
		return []FieldError{{Field: field, Reason: "must be a finite number"}}
	}
	if *v < min || *v > max {
		return []FieldError{{Field: field, Reason: fmt.Sprintf("must be between %g and %g", min, max)}}
	}
	return nil
}

func validateLat(field string, v float64) []FieldError {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return []FieldError{{Field: field, Reason: "must be a finite number"}}
	}
	if v < worldLatMin || v > worldLatMax {
		return []FieldError{{Field: field, Reason: "must be between -90 and 90"}}
	}
	return nil
}

func validateLng(field string, v float64) []FieldError {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return []FieldError{{Field: field, Reason: "must be a finite number"}}
	}
	if v < worldLngMin || v > worldLngMax {
		return []FieldError{{Field: field, Reason: "must be between -180 and 180"}}
	}
	return nil
}

func validateTags(tags models.PlaceTags) []FieldError {
	if len(tags) > maxTagEntries {
		return []FieldError{{Field: "tags", Reason: fmt.Sprintf("must contain ≤ %d entries", maxTagEntries)}}
	}
	var errs []FieldError
	for k, v := range tags {
		if len(k) > maxTagKeyLength {
			errs = append(errs, FieldError{Field: "tags", Reason: fmt.Sprintf("key %q exceeds %d characters", k, maxTagKeyLength)})
		}
		if len(v) > maxTagValueLength {
			errs = append(errs, FieldError{Field: "tags", Reason: fmt.Sprintf("value for key %q exceeds %d characters", k, maxTagValueLength)})
		}
	}
	return errs
}

func validateMetadata(md map[string]any) []FieldError {
	if len(md) > maxMetadataEntries {
		return []FieldError{{Field: "metadata", Reason: fmt.Sprintf("must contain ≤ %d entries", maxMetadataEntries)}}
	}
	if len(md) == 0 {
		return nil
	}
	b, err := json.Marshal(md)
	if err != nil {
		return []FieldError{{Field: "metadata", Reason: "must be JSON-serialisable"}}
	}
	if len(b) > maxMetadataBytes {
		return []FieldError{{Field: "metadata", Reason: fmt.Sprintf("serialised size exceeds %d bytes", maxMetadataBytes)}}
	}
	return nil
}

func parseFloat(field, raw string) (float64, *FieldError) {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, &FieldError{Field: field, Reason: "must be a finite number"}
	}
	return v, nil
}

func nonEmptyCount(ss []string) int {
	n := 0
	for _, s := range ss {
		if s != "" {
			n++
		}
	}
	return n
}

func appendIfErr(dst []FieldError, errs ...*FieldError) []FieldError {
	for _, e := range errs {
		if e != nil {
			dst = append(dst, *e)
		}
	}
	return dst
}
