/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

// Package validation enforces constraints that cannot be expressed in the
// OpenAPI spec: whitespace-only strings, tag/metadata size limits, mutual
// exclusivity of query-param groups, and cursor format.
// Structural checks (required fields, enum values, numeric bounds, UUID format)
// are handled by the nethttp-middleware spec validator before handlers run.
package validation

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/InWheelOrg/inwheel-server/internal/pagination"
	"github.com/InWheelOrg/inwheel-server/pkg/models"
)

// FieldError describes a single rejected field in a 400 response.
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
	maxParkingCount    = 10000
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// Email validates that s is a syntactically well-formed email address.
func Email(s string) []FieldError {
	if !emailRegex.MatchString(s) {
		return []FieldError{{Field: "email", Reason: "must be a valid email address"}}
	}
	return nil
}

// Place validates fields that the OpenAPI spec cannot express: a whitespace-only
// name, oversized tags, and oversized metadata on inline accessibility components.
func Place(p *models.Place) []FieldError {
	if p == nil {
		return []FieldError{{Field: "body", Reason: "place is required"}}
	}

	var errs []FieldError

	if strings.TrimSpace(p.Name) == "" {
		errs = append(errs, FieldError{Field: "name", Reason: "must not be blank"})
	}

	errs = append(errs, validateTags(p.Tags)...)

	if p.Accessibility != nil {
		for _, comp := range p.Accessibility.Components {
			errs = append(errs, validateMetadata(comp.Metadata)...)
		}
	}

	return errs
}

// PlacesQuery validates constraints on GET /places query params that OpenAPI
// cannot express: mutual exclusivity of proximity vs bbox param groups,
// bounding-box ordering (min < max), and cursor internal format.
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
		return []FieldError{{Field: "query", Reason: "proximity and bounding-box parameters are mutually exclusive"}}
	}

	var errs []FieldError

	if bboxPresent == len(bbox) {
		minLng, e1 := strconv.ParseFloat(q.Get("min_lng"), 64)
		minLat, e2 := strconv.ParseFloat(q.Get("min_lat"), 64)
		maxLng, e3 := strconv.ParseFloat(q.Get("max_lng"), 64)
		maxLat, e4 := strconv.ParseFloat(q.Get("max_lat"), 64)

		if e1 == nil && e3 == nil && minLng >= maxLng {
			errs = append(errs, FieldError{Field: "min_lng", Reason: "must be less than max_lng"})
		}
		if e2 == nil && e4 == nil && minLat >= maxLat {
			errs = append(errs, FieldError{Field: "min_lat", Reason: "must be less than max_lat"})
		}
		_ = e1
		_ = e2
		_ = e3
		_ = e4
	}

	if c := q.Get("cursor"); c != "" {
		if _, _, err := pagination.Decode(c); err != nil {
			errs = append(errs, FieldError{Field: "cursor", Reason: "invalid cursor"})
		}
	}

	return errs
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

func nonEmptyCount(ss []string) int {
	n := 0
	for _, s := range ss {
		if s != "" {
			n++
		}
	}
	return n
}
