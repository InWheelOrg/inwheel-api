/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package db

import (
	"fmt"
	"log"
	"time"

	"github.com/InWheelOrg/inwheel-server/pkg/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Config holds the database connection configuration.
type Config struct {
	Host         string
	Port         int
	User         string
	Password     string
	Name         string
	SSLMode      string
	MaxOpenConns int
	MaxIdleConns int
}

// Connect initializes the database connection.
func Connect(cfg Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=%s",
		cfg.Host, cfg.User, cfg.Password, cfg.Name, cfg.Port, cfg.SSLMode)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("could not get generic db: %w", err)
	}

	maxOpen := cfg.MaxOpenConns
	if maxOpen == 0 {
		maxOpen = 25
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle == 0 {
		maxIdle = 5
	}

	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}

// Migrate performs the database schema migrations.
func Migrate(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database connection not initialized")
	}

	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS postgis").Error; err != nil {
		log.Printf("Warning: Failed to ensure PostGIS extension: %v", err)
	}
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS \"pgcrypto\"").Error; err != nil {
		log.Printf("Warning: Failed to ensure pgcrypto extension: %v", err)
	}

	indexQuery := "CREATE INDEX IF NOT EXISTS idx_places_geog ON places USING GIST (geography(ST_Point(lng, lat)))"
	if err := db.Exec(indexQuery).Error; err != nil {
		log.Printf("Warning: Failed to create spatial index: %v", err)
	}

	return db.AutoMigrate(&models.Place{}, &models.AccessibilityProfile{})
}
