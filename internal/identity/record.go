/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package identity

import (
	"encoding/json"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// Record is one incoming non-OSM row being matched against the places table.
// Ingestion source adapters (e.g. Wheelmap) construct a Record at the package
// boundary from their upstream format.
type Record struct {
	// Source identifies the upstream system, e.g. "wheelmap".
	Source string
	// SourceID is the upstream's identifier for this row.
	SourceID string
	// Name is the human-readable name of the place.
	Name string
	// Lat is the latitude of the place.
	Lat float64
	// Lng is the longitude of the place.
	Lng float64
	// Category is the InWheel category the source mapped this row to.
	Category models.Category
	// Street is the normalized street name, empty if absent in the source.
	Street string
	// HouseNumber is the building number, empty if absent in the source.
	HouseNumber string
	// Payload is the raw upstream row, persisted to unmatched_external when
	// no match is found so a retry sweep can re-evaluate it later.
	Payload json.RawMessage
}
