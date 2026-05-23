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

	internaldb "github.com/InWheelOrg/inwheel-api/internal/db"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"
)

// ConnInfo holds the raw connection parameters for a test container.
// Use it when you need to open a second, independent database connection
// (e.g. to test code paths that manage their own connection lifecycle).
type ConnInfo struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
}

// StartPostgres starts a PostGIS container, runs migrations, and returns a connected *gorm.DB.
// The returned cleanup function terminates the container and must be called when tests are done.
func StartPostgres(ctx context.Context) (*gorm.DB, func(), error) {
	db, _, cleanup, err := startPostgresInner(ctx)
	return db, cleanup, err
}

// StartPostgresWithConnInfo is like StartPostgres but additionally returns the raw
// connection parameters so callers can open their own independent connections.
func StartPostgresWithConnInfo(ctx context.Context) (*gorm.DB, ConnInfo, func(), error) {
	return startPostgresInner(ctx)
}

// startPostgresInner is the shared implementation.
func startPostgresInner(ctx context.Context) (*gorm.DB, ConnInfo, func(), error) {
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
		return nil, ConnInfo{}, nil, fmt.Errorf("start postgres container: %w", err)
	}

	cleanup := func() {
		container.Terminate(ctx)
	}

	host, err := container.Host(ctx)
	if err != nil {
		cleanup()
		return nil, ConnInfo{}, nil, fmt.Errorf("get container host: %w", err)
	}

	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		cleanup()
		return nil, ConnInfo{}, nil, fmt.Errorf("get container port: %w", err)
	}

	info := ConnInfo{
		Host:     host,
		Port:     int(port.Num()),
		User:     "test",
		Password: "test",
		Name:     "inwheel_test",
		SSLMode:  "disable",
	}

	gormDB, err := internaldb.Connect(internaldb.Config{
		Host:     info.Host,
		Port:     info.Port,
		User:     info.User,
		Password: info.Password,
		Name:     info.Name,
		SSLMode:  info.SSLMode,
	})
	if err != nil {
		cleanup()
		return nil, ConnInfo{}, nil, fmt.Errorf("connect to test db: %w", err)
	}

	if err := internaldb.Migrate(gormDB); err != nil {
		cleanup()
		return nil, ConnInfo{}, nil, fmt.Errorf("migrate test db: %w", err)
	}

	return gormDB, info, cleanup, nil
}
