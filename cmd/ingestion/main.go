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

// runPipeline executes one ingest of src against gormDB. Canonical sources go
// through the batched upsert path; external sources go through identity.Resolver.
func runPipeline(ctx context.Context, src sources.Source, command string, gormDB *gorm.DB) error {
	placesRepo := place.NewRepository(gormDB)
	unmatchedRepo := unmatched.NewRepository(gormDB)

	b := &batcher{size: batchSize, flush: placesRepo.UpsertBatch}

	resolver := &identity.Resolver{
		Candidates: placesRepo,
		Places:     placesRepo,
		Unmatched:  unmatchedRepo,
		Now:        time.Now,
	}
	counters := &resolveCounters{}
	recordSink := buildRecordSink(resolver, counters)

	if err := dispatch(ctx, src, command, b.sink, recordSink); err != nil {
		return fmt.Errorf("source %q: %w", src.Name(), err)
	}
	if err := b.flushNow(ctx); err != nil {
		return fmt.Errorf("final flush: %w", err)
	}
	args := []any{"source", src.Name(), "command", command}
	switch src.Kind() {
	case sources.SourceKindCanonical:
		args = append(args, "written", b.written)
	case sources.SourceKindExternal:
		args = append(args,
			"confident", counters.confident,
			"low_confidence", counters.lowConfidence,
			"no_match", counters.noMatch,
			"errors", counters.errors,
		)
	}
	slog.Info("ingestion complete", args...)
	return nil
}

func dispatch(ctx context.Context, src sources.Source, command string, sink sources.Sink, recordSink sources.RecordSink) error {
	switch src.Kind() {
	case sources.SourceKindCanonical:
		return dispatchCanonical(ctx, src, command, sink)
	case sources.SourceKindExternal:
		return dispatchExternal(ctx, src, command, recordSink)
	default:
		return fmt.Errorf("source %q has unknown kind: %v", src.Name(), src.Kind())
	}
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
