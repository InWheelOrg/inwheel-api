//go:build integration

/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package auditor

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/InWheelOrg/inwheel-server/internal/testhelpers"
	"github.com/InWheelOrg/inwheel-server/pkg/models"
	"gorm.io/gorm"
)

var testDB *gorm.DB

// mockAuditor implements llmAuditor for testing without a real Ollama instance.
// When bumpVersionOnCall is true, it simulates a concurrent PATCH by incrementing
// data_version in the DB mid-flight, which is the condition the version-check guard protects against.
type mockAuditor struct {
	result            *models.AuditResult
	bumpVersionOnCall bool
	db                *gorm.DB
}

func (m *mockAuditor) audit(_ context.Context, profile *models.AccessibilityProfile) (*models.AuditResult, error) {
	if m.bumpVersionOnCall {
		m.db.Model(&models.AccessibilityProfile{}).
			Where("id = ?", profile.ID).
			Update("data_version", profile.DataVersion+1)
	}
	return m.result, nil
}

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

// run is extracted from TestMain so defer-based cleanup executes before os.Exit.
func run(m *testing.M) int {
	ctx := context.Background()
	var cleanup func()
	var err error

	testDB, cleanup, err = testhelpers.StartPostgres(ctx)
	if err != nil {
		log.Fatalf("start test postgres: %v", err)
	}
	defer cleanup()

	return m.Run()
}

// truncate clears all test data between tests to prevent state bleed.
func truncate(t *testing.T) {
	t.Helper()
	testDB.Exec("TRUNCATE places, accessibility_profiles CASCADE")
}

// seedProfile inserts a place and a linked accessibility profile marked as needing audit.
func seedProfile(t *testing.T) models.AccessibilityProfile {
	t.Helper()

	place := models.Place{Name: "Test", Lat: 52.5, Lng: 13.4, Category: "cafe", Source: "test"}
	if err := testDB.Create(&place).Error; err != nil {
		t.Fatalf("seed place: %v", err)
	}

	profile := models.AccessibilityProfile{
		PlaceID:       place.ID,
		OverallStatus: models.StatusAccessible,
		NeedsAudit:    true,
		DataVersion:   1,
		UpdatedAt:     time.Now(),
	}
	if err := testDB.Create(&profile).Error; err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	return profile
}

func TestProcessNextTask_VersionMismatch_DiscardsResult(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	seeded := seedProfile(t)

	a := &Auditor{
		db: testDB,
		llm: &mockAuditor{
			result:            &models.AuditResult{HasConflict: false, Confidence: 0.9},
			bumpVersionOnCall: true,
			db:                testDB,
		},
	}

	processed, err := a.ProcessNextTask(context.Background())
	if err != nil {
		t.Fatalf("ProcessNextTask error: %v", err)
	}
	if !processed {
		t.Fatal("expected task to be claimed and attempted")
	}

	var profile models.AccessibilityProfile
	testDB.First(&profile, "id = ?", seeded.ID)

	if !profile.NeedsAudit {
		t.Error("expected NeedsAudit=true after version mismatch (result should be discarded and task re-queued)")
	}
	if profile.Audit != nil {
		t.Error("expected audit result to be nil after version mismatch")
	}
}

func TestProcessNextTask_HappyPath_SavesResult(t *testing.T) {
	t.Cleanup(func() { truncate(t) })

	seeded := seedProfile(t)

	a := &Auditor{
		db: testDB,
		llm: &mockAuditor{
			result: &models.AuditResult{
				HasConflict: true,
				Reasoning:   "entrance is inaccessible but overall status is accessible",
				Confidence:  0.95,
			},
			bumpVersionOnCall: false,
		},
	}

	processed, err := a.ProcessNextTask(context.Background())
	if err != nil {
		t.Fatalf("ProcessNextTask error: %v", err)
	}
	if !processed {
		t.Fatal("expected task to be claimed and processed")
	}

	var profile models.AccessibilityProfile
	testDB.First(&profile, "id = ?", seeded.ID)

	if profile.NeedsAudit {
		t.Error("expected NeedsAudit=false after successful audit")
	}
	if profile.Audit == nil {
		t.Fatal("expected audit result to be saved")
	}
	if !profile.Audit.HasConflict {
		t.Error("expected HasConflict=true from fake LLM result")
	}
}
