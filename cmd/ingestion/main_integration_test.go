//go:build integration

/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"context"
	"testing"

	"github.com/InWheelOrg/inwheel-api/internal/testhelpers"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

func TestRunFullImport_AgainstFixturePBF(t *testing.T) {
	ctx := context.Background()
	db, connInfo, cleanup, err := testhelpers.StartPostgresWithConnInfo(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer cleanup()

	cfg := config{
		DBHost:     connInfo.Host,
		DBPort:     connInfo.Port,
		DBUser:     connInfo.User,
		DBPassword: connInfo.Password,
		DBName:     connInfo.Name,
		DBSSLMode:  connInfo.SSLMode,
		OSMPBFPath: "../../testdata/andorra-sample.osm.pbf",
	}

	if err := runFullImport(ctx, cfg); err != nil {
		t.Fatalf("runFullImport: %v", err)
	}

	var count int64
	if err := db.Model(&models.Place{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	t.Logf("imported %d places from the Andorra fixture", count)
	if count == 0 {
		t.Fatal("expected at least one place row, got zero")
	}
}
