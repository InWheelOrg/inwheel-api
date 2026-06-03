/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"fmt"

	"github.com/InWheelOrg/inwheel-api/internal/sources"
	"github.com/InWheelOrg/inwheel-api/internal/sources/osm"
)

// buildSource returns the concrete sources.Source for the given name,
// constructed from cfg. Unknown names and missing source-specific config
// produce an error.
func buildSource(name string, cfg config) (sources.Source, error) {
	switch name {
	case "osm":
		if cfg.OSMPBFPath == "" {
			return nil, fmt.Errorf("source %q requires OSM_PBF_PATH", name)
		}
		return &osm.Source{PBFPath: cfg.OSMPBFPath}, nil
	default:
		return nil, fmt.Errorf("unknown source: %q", name)
	}
}
