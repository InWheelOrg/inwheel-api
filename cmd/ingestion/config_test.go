/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"strings"
	"testing"
)

func TestLoadConfig_AllValuesProvided(t *testing.T) {
	env := map[string]string{
		"DB_HOST":      "db.example.com",
		"DB_PORT":      "6543",
		"DB_USER":      "inwheel_user",
		"DB_PASSWORD":  "s3cret",
		"DB_NAME":      "inwheel_prod",
		"DB_SSLMODE":   "require",
		"OSM_PBF_PATH": "/data/finland.osm.pbf",
	}

	cfg, err := loadConfig(env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DBHost != "db.example.com" || cfg.DBPort != 6543 || cfg.DBUser != "inwheel_user" {
		t.Errorf("unexpected DB config: %+v", cfg)
	}
	if cfg.OSMPBFPath != "/data/finland.osm.pbf" {
		t.Errorf("OSM_PBF_PATH not honored: %q", cfg.OSMPBFPath)
	}
}

func TestLoadConfig_AppliesDefaults(t *testing.T) {
	env := map[string]string{"OSM_PBF_PATH": "/tmp/a.pbf"}

	cfg, err := loadConfig(env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DBHost != "localhost" || cfg.DBPort != 5432 || cfg.DBUser != "postgres" ||
		cfg.DBPassword != "postgres" || cfg.DBName != "inwheel" || cfg.DBSSLMode != "disable" {
		t.Errorf("defaults not applied: %+v", cfg)
	}
}

func TestLoadConfig_MissingOSMPath(t *testing.T) {
	_, err := loadConfig(map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing OSM_PBF_PATH, got nil")
	}
	if !strings.Contains(err.Error(), "OSM_PBF_PATH") {
		t.Errorf("error should mention OSM_PBF_PATH: %v", err)
	}
}

func TestLoadConfig_MalformedPort(t *testing.T) {
	env := map[string]string{
		"OSM_PBF_PATH": "/tmp/a.pbf",
		"DB_PORT":      "not-a-number",
	}
	_, err := loadConfig(env)
	if err == nil {
		t.Fatal("expected error for malformed DB_PORT, got nil")
	}
	if !strings.Contains(err.Error(), "DB_PORT") {
		t.Errorf("error should mention DB_PORT: %v", err)
	}
}

func TestLoadConfig_AccumulatesMultipleErrors(t *testing.T) {
	env := map[string]string{"DB_PORT": "bad"}

	_, err := loadConfig(env)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "OSM_PBF_PATH") || !strings.Contains(msg, "DB_PORT") {
		t.Errorf("expected both errors, got: %v", err)
	}
}
