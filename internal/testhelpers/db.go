//go:build integration

/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package testhelpers

import (
	"context"
	"fmt"
	"time"

	internaldb "github.com/InWheelOrg/inwheel-server/internal/db"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"
)

// StartPostgres starts a PostGIS container, runs migrations, and returns a connected *gorm.DB.
// The returned cleanup function terminates the container and must be called when tests are done.
func StartPostgres(ctx context.Context) (*gorm.DB, func(), error) {
	container, err := tcpostgres.Run(ctx, "postgis/postgis:18-3.6",
		tcpostgres.WithDatabase("inwheel_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(2*time.Minute),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("start postgres container: %w", err)
	}

	cleanup := func() {
		container.Terminate(ctx)
	}

	host, err := container.Host(ctx)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("get container host: %w", err)
	}

	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("get container port: %w", err)
	}

	gormDB, err := internaldb.Connect(internaldb.Config{
		Host:     host,
		Port:     int(port.Num()),
		User:     "test",
		Password: "test",
		Name:     "inwheel_test",
		SSLMode:  "disable",
	})
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("connect to test db: %w", err)
	}

	if err := internaldb.Migrate(gormDB); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("migrate test db: %w", err)
	}

	return gormDB, cleanup, nil
}
