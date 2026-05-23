/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"fmt"
	"strconv"
	"strings"
)

const defaultDBPort = 5432

// config holds the configuration for cmd/ingestion. All values come from environment variables.
type config struct {
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string
	OSMPBFPath string
}

// loadConfig reads configuration from the given map (typically os.Environ wrapped as a map).
// Required variables that are absent or malformed produce a single error listing every problem.
func loadConfig(env map[string]string) (config, error) {
	var errs []string

	osmPath := env["OSM_PBF_PATH"]
	if strings.TrimSpace(osmPath) == "" {
		errs = append(errs, "OSM_PBF_PATH is required but was not set")
	}

	port := defaultDBPort
	if raw, ok := env["DB_PORT"]; ok && raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			errs = append(errs, fmt.Sprintf("DB_PORT must be an integer, got: %q", raw))
		} else {
			port = parsed
		}
	}

	if len(errs) > 0 {
		return config{}, fmt.Errorf("invalid configuration:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return config{
		DBHost:     valueOr(env, "DB_HOST", "localhost"),
		DBPort:     port,
		DBUser:     valueOr(env, "DB_USER", "postgres"),
		DBPassword: valueOr(env, "DB_PASSWORD", "postgres"),
		DBName:     valueOr(env, "DB_NAME", "inwheel"),
		DBSSLMode:  valueOr(env, "DB_SSLMODE", "disable"),
		OSMPBFPath: osmPath,
	}, nil
}

func valueOr(env map[string]string, key, fallback string) string {
	if v, ok := env[key]; ok && v != "" {
		return v
	}
	return fallback
}
