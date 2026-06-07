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
	"time"

	"gorm.io/gorm"

	"github.com/InWheelOrg/inwheel-api/internal/db"
	"github.com/InWheelOrg/inwheel-api/internal/identity"
	"github.com/InWheelOrg/inwheel-api/internal/place"
	"github.com/InWheelOrg/inwheel-api/internal/sources"
	"github.com/InWheelOrg/inwheel-api/internal/unmatched"
)

const batchSize = 1000

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: inwheel-ingestion <source> <full-import|diff-sync>")
		os.Exit(1)
	}
	sourceName := os.Args[1]
	command := os.Args[2]

	cfg, err := loadConfig(environAsMap())
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}
	slog.Info("config loaded",
		"source", sourceName,
		"command", command,
		"db.host", cfg.DBHost,
		"db.port", cfg.DBPort,
		"db.user", cfg.DBUser,
		"db.name", cfg.DBName,
		"db.sslmode", cfg.DBSSLMode,
		"osm.pbf_path", cfg.OSMPBFPath,
	)

	if err := run(context.Background(), sourceName, command, cfg); err != nil {
		slog.Error("ingestion failed", "source", sourceName, "command", command, "error", err)
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

func run(ctx context.Context, sourceName, command string, cfg config) error {
	src, err := buildSource(sourceName, cfg)
	if err != nil {
		return err
	}
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
	return runPipeline(ctx, src, command, gormDB)
}

// runPipeline routes src to its pipeline based on Kind.
func runPipeline(ctx context.Context, src sources.Source, command string, gormDB *gorm.DB) error {
	switch src.Kind() {
	case sources.SourceKindCanonical:
		return runCanonical(ctx, src, command, gormDB)
	case sources.SourceKindExternal:
		return runExternal(ctx, src, command, gormDB)
	default:
		return fmt.Errorf("source %q has unknown kind: %v", src.Name(), src.Kind())
	}
}

// runCanonical drives a canonical source through the batched upsert path,
// then runs the retry sweep against the IDs of places the batcher touched.
func runCanonical(ctx context.Context, src sources.Source, command string, gormDB *gorm.DB) error {
	placesRepo := place.NewRepository(gormDB)
	unmatchedRepo := unmatched.NewRepository(gormDB)
	b := &batcher{size: batchSize, flush: placesRepo.UpsertBatch}
	if err := dispatchCanonical(ctx, src, command, b.sink); err != nil {
		return fmt.Errorf("source %q: %w", src.Name(), err)
	}
	if err := b.flushNow(ctx); err != nil {
		return fmt.Errorf("final flush: %w", err)
	}

	sweeper := &identity.Sweeper{
		Candidates: placesRepo,
		Places:     placesRepo,
		Queue:      unmatchedRepo,
		Now:        time.Now,
	}
	sweepResult, sweepErr := sweeper.Sweep(ctx, b.touchedIDs)
	if sweepErr != nil {
		slog.Warn("sweep failed",
			"source", src.Name(),
			"command", command,
			"error", sweepErr,
		)
	}

	slog.Info("ingestion complete",
		"source", src.Name(),
		"command", command,
		"written", b.written,
		"sweep_considered", sweepResult.Considered,
		"sweep_confident", sweepResult.Confident,
		"sweep_low_confidence", sweepResult.LowConfidence,
		"sweep_no_match", sweepResult.NoMatch,
		"sweep_errors", sweepResult.Errors,
	)
	return nil
}

// runExternal drives an external source through identity.Resolver, attaching
// matched external refs and queueing the rest.
func runExternal(ctx context.Context, src sources.Source, command string, gormDB *gorm.DB) error {
	placesRepo := place.NewRepository(gormDB)
	unmatchedRepo := unmatched.NewRepository(gormDB)
	resolver := &identity.Resolver{
		Candidates: placesRepo,
		Places:     placesRepo,
		Unmatched:  unmatchedRepo,
		Now:        time.Now,
	}
	counters := &resolveCounters{}
	sink := buildRecordSink(resolver, counters)
	if err := dispatchExternal(ctx, src, command, sink); err != nil {
		return fmt.Errorf("source %q: %w", src.Name(), err)
	}
	slog.Info("ingestion complete",
		"source", src.Name(),
		"command", command,
		"confident", counters.confident,
		"low_confidence", counters.lowConfidence,
		"no_match", counters.noMatch,
		"errors", counters.errors,
	)
	return nil
}

func dispatchCanonical(ctx context.Context, src sources.Source, command string, sink sources.Sink) error {
	switch command {
	case "full-import":
		fi, ok := src.(sources.FullImporter)
		if !ok {
			return fmt.Errorf("source %q does not support full-import", src.Name())
		}
		return fi.FullImport(ctx, sink)
	case "diff-sync":
		ds, ok := src.(sources.DiffSyncer)
		if !ok {
			return fmt.Errorf("source %q does not support diff-sync", src.Name())
		}
		return ds.DiffSync(ctx, time.Time{}, sink)
	default:
		return fmt.Errorf("unknown command: %q", command)
	}
}

func dispatchExternal(ctx context.Context, src sources.Source, command string, sink sources.RecordSink) error {
	switch command {
	case "full-import":
		fi, ok := src.(sources.ExternalFullImporter)
		if !ok {
			return fmt.Errorf("source %q does not support full-import", src.Name())
		}
		return fi.FullImport(ctx, sink)
	case "diff-sync":
		ds, ok := src.(sources.ExternalDiffSyncer)
		if !ok {
			return fmt.Errorf("source %q does not support diff-sync", src.Name())
		}
		return ds.DiffSync(ctx, time.Time{}, sink)
	default:
		return fmt.Errorf("unknown command: %q", command)
	}
}

// resolveCounters tallies external-source outcomes for the run summary.
type resolveCounters struct {
	confident     int
	lowConfidence int
	noMatch       int
	errors        int
}

// buildRecordSink returns a RecordSink that runs resolver on each record,
// tallies outcomes into counters, and logs but does not abort on per-record errors.
func buildRecordSink(resolver *identity.Resolver, counters *resolveCounters) sources.RecordSink {
	return func(ctx context.Context, r identity.Record) error {
		d, err := resolver.Resolve(ctx, r)
		if err != nil {
			counters.errors++
			slog.Warn("resolve failed",
				"source", r.Source,
				"source_id", r.SourceID,
				"error", err,
			)
			return nil
		}
		switch d.Kind {
		case identity.KindConfident:
			counters.confident++
		case identity.KindLowConfidence:
			counters.lowConfidence++
		case identity.KindNoMatch:
			counters.noMatch++
		}
		return nil
	}
}
