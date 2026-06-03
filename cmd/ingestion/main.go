/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/InWheelOrg/inwheel-api/internal/db"
	"github.com/InWheelOrg/inwheel-api/internal/sources/osm"
	"github.com/InWheelOrg/inwheel-api/internal/place"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

const batchSize = 1000

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: inwheel-ingestion <full-import|diff-sync>")
		os.Exit(1)
	}

	cfg, err := loadConfig(environAsMap())
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}
	slog.Info("config loaded",
		"db.host", cfg.DBHost,
		"db.port", cfg.DBPort,
		"db.user", cfg.DBUser,
		"db.name", cfg.DBName,
		"db.sslmode", cfg.DBSSLMode,
		"osm.pbf_path", cfg.OSMPBFPath,
	)

	switch os.Args[1] {
	case "full-import":
		if err := runFullImport(context.Background(), cfg); err != nil {
			slog.Error("full-import failed", "error", err)
			os.Exit(1)
		}
	case "diff-sync":
		slog.Error("diff-sync not implemented yet")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func environAsMap() map[string]string {
	out := make(map[string]string, len(os.Environ()))
	for _, kv := range os.Environ() {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				out[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	return out
}

func runFullImport(ctx context.Context, cfg config) error {
	gormDB, err := db.Connect(db.Config{
		Host:     cfg.DBHost,
		Port:     cfg.DBPort,
		User:     cfg.DBUser,
		Password: cfg.DBPassword,
		Name:     cfg.DBName,
		SSLMode:  cfg.DBSSLMode,
	})
	if err != nil {
		return fmt.Errorf("db connect: %w", err)
	}
	if err := db.Migrate(gormDB); err != nil {
		return fmt.Errorf("db migrate: %w", err)
	}

	repo := place.NewRepository(gormDB)

	f, err := os.Open(cfg.OSMPBFPath)
	if err != nil {
		return fmt.Errorf("open pbf: %w", err)
	}
	defer f.Close() //nolint:errcheck

	var (
		buffer    []models.Place
		processed int
		written   int
	)

	flush := func() error {
		if len(buffer) == 0 {
			return nil
		}
		if err := repo.UpsertBatch(ctx, buffer); err != nil {
			return err
		}
		written += len(buffer)
		buffer = buffer[:0]
		return nil
	}

	err = osm.StreamNodes(ctx, f, func(node osm.Node) error {
		processed++
		if processed%10000 == 0 {
			slog.Info("progress", "processed", processed, "written", written)
		}

		category, ok := osm.Evaluate(node.Tags)
		if !ok {
			return nil
		}

		p, err := osm.TransformNode(node.ID, node.Lat, node.Lng, node.Tags, category)
		if err != nil {
			slog.Warn("skipping node, transform error", "node_id", node.ID, "error", err)
			return nil
		}

		buffer = append(buffer, *p)
		if len(buffer) >= batchSize {
			return flush()
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("stream: %w", err)
	}

	if err := flush(); err != nil {
		return fmt.Errorf("final flush: %w", err)
	}

	slog.Info("full-import complete", "processed", processed, "written", written)
	return nil
}
