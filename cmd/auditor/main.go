/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/InWheelOrg/inwheel-server/internal/auditor"
	"github.com/InWheelOrg/inwheel-server/internal/db"
	"github.com/ollama/ollama/api"
)

func main() {
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort, _ := strconv.Atoi(getEnv("DB_PORT", "5432"))
	dbUser := getEnv("DB_USER", "postgres")
	dbPass := getEnv("DB_PASSWORD", "postgres")
	dbName := getEnv("DB_NAME", "inwheel")
	dbSSL := getEnv("DB_SSLMODE", "disable")
	dbMaxOpen, _ := strconv.Atoi(getEnv("DB_MAX_OPEN_CONNS", "5"))
	dbMaxIdle, _ := strconv.Atoi(getEnv("DB_MAX_IDLE_CONNS", "2"))

	ollamaURL := getEnv("OLLAMA_URL", "http://localhost:11434")
	model := getEnv("AUDIT_MODEL", "deepseek-r1:1.5b")

	gormDB, err := db.Connect(db.Config{
		Host:         dbHost,
		Port:         dbPort,
		User:         dbUser,
		Password:     dbPass,
		Name:         dbName,
		SSLMode:      dbSSL,
		MaxOpenConns: dbMaxOpen,
		MaxIdleConns: dbMaxIdle,
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := db.Migrate(gormDB); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	parsedURL, err := url.Parse(ollamaURL)
	if err != nil {
		log.Fatalf("Invalid OLLAMA_URL: %v", err)
	}

	httpClient := &http.Client{
		Timeout: 2 * time.Minute,
	}
	ollamaClient := api.NewClient(parsedURL, httpClient)

	worker := auditor.NewAuditor(gormDB, ollamaClient, model)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Println("Shutdown signal received. Gracefully stopping Auditor service...")
	}()

	log.Printf("Starting Auditor service (model: %s)...", model)
	worker.Start(ctx)
	log.Println("Auditor service stopped gracefully.")
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
