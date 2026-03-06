/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package auditor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/InWheelOrg/inwheel-server/pkg/models"
	"github.com/ollama/ollama/api"
	"gorm.io/gorm"
)

// Auditor handles the background accessibility audit tasks.
type Auditor struct {
	db     *gorm.DB
	ollama *api.Client
	model  string
}

// NewAuditor creates a new Auditor service.
func NewAuditor(db *gorm.DB, ollama *api.Client, model string) *Auditor {
	return &Auditor{
		db:     db,
		ollama: ollama,
		model:  model,
	}
}

// Start runs the auditor loop.
func (a *Auditor) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			processed, err := a.ProcessNextTask(ctx)
			if err != nil {
				log.Printf("Audit error: %v. Backing off...", err)
				time.Sleep(30 * time.Second)
				continue
			}

			if !processed {
				select {
				case <-ctx.Done():
					return
				case <-time.After(10 * time.Second):
					continue
				}
			}
		}
	}
}

// ProcessNextTask claims one pending task and processes it.
func (a *Auditor) ProcessNextTask(ctx context.Context) (bool, error) {
	// Add a 3-minute timeout for the entire task processing, including the LLM request.
	// This should be longer than the HTTP client timeout (2m) to allow for some overhead.
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	var profile models.AccessibilityProfile

	err := a.db.Transaction(func(tx *gorm.DB) error {
		claimSQL := `
			UPDATE accessibility_profiles 
			SET audit_locked_until = NOW() + INTERVAL '2 minutes'
			WHERE id = (
				SELECT id FROM accessibility_profiles 
				WHERE needs_audit = true 
				AND (audit_locked_until IS NULL OR audit_locked_until < NOW())
				ORDER BY priority DESC, updated_at ASC
				LIMIT 1
				FOR UPDATE SKIP LOCKED
			)
			RETURNING *;
		`
		if err := tx.Raw(claimSQL).Scan(&profile).Error; err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return false, err
	}

	if profile.ID == "" {
		return false, nil // No tasks found
	}

	result, err := a.auditWithLLM(ctx, &profile)
	if err != nil {
		// Unlock on failure so it can be retried later
		a.db.Model(&profile).Update("audit_locked_until", nil)
		return false, fmt.Errorf("LLM audit failed: %w", err)
	}

	err = a.db.Transaction(func(tx *gorm.DB) error {
		var current models.AccessibilityProfile
		if err := tx.Where("id = ?", profile.ID).First(&current).Error; err != nil {
			return err
		}

		// If data_version has changed, discard the LLM result as it's based on outdated data
		if current.DataVersion > profile.DataVersion {
			return tx.Model(&current).Update("needs_audit", true).Error
		}

		return tx.Model(&current).Updates(map[string]any{
			"audit":              result,
			"needs_audit":        false,
			"audit_locked_until": nil,
		}).Error
	})

	if err != nil {
		return false, err
	}

	return true, nil
}

func (a *Auditor) auditWithLLM(ctx context.Context, profile *models.AccessibilityProfile) (*models.AuditResult, error) {
	prompt := `You are an Accessibility Auditor. Your goal is to detect contradictions in a nested JSON structure.

Rules for Components:
- Check each 'A11yComponent'. If its technical properties (e.g., 'entrance.step_height' > 0.05) 
  contradict its 'overall_status' (e.g., 'accessible'), FLAG CONFLICT.
- For 'restroom', if 'wheelchair_accessible' is false but 'overall_status' is 'accessible', FLAG CONFLICT.

Rules for the Profile:
- Compare the 'overall_status' of the entire profile against the 'overall_status' of its components.
- If a critical component (like 'entrance') is 'inaccessible', the profile's 'overall_status' 
  cannot be 'accessible'.

Respond ONLY in JSON: {"has_conflict": bool, "reasoning": "string", "confidence": float}.`

	profileJSON, err := json.Marshal(profile)
	if err != nil {
		return nil, err
	}

	req := &api.GenerateRequest{
		Model:  a.model,
		Prompt: fmt.Sprintf("%s\n\nInput Profile: %s", prompt, string(profileJSON)),
		Format: json.RawMessage(`"json"`),
		Options: map[string]any{
			"temperature": 0.0,
			"num_ctx":     2048,
		},
		Stream: new(bool), // No streaming
	}

	var auditResponse struct {
		HasConflict bool    `json:"has_conflict"`
		Reason      string  `json:"reason"`
		Confidence  float64 `json:"confidence"`
	}

	err = a.ollama.Generate(ctx, req, func(resp api.GenerateResponse) error {
		return json.Unmarshal([]byte(resp.Response), &auditResponse)
	})
	if err != nil {
		return nil, err
	}

	return &models.AuditResult{
		HasConflict: auditResponse.HasConflict,
		Reasoning:   auditResponse.Reason,
		Confidence:  auditResponse.Confidence,
		LastAudit:   time.Now().Format(time.RFC3339),
	}, nil
}
