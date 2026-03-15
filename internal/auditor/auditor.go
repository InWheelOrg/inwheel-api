/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package auditor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/InWheelOrg/inwheel-server/internal/a11y"
	"github.com/InWheelOrg/inwheel-server/pkg/models"
	"github.com/ollama/ollama/api"
	"gorm.io/gorm"
)

// llmAuditor is the interface for running an accessibility audit against a language model.
type llmAuditor interface {
	audit(ctx context.Context, profile *models.AccessibilityProfile) (*models.AuditResult, error)
}

// ollamaAuditor is the production implementation of llmAuditor backed by a local Ollama instance.
type ollamaAuditor struct {
	client *api.Client
	model  string
	engine *a11y.Engine
}

// Auditor handles the background accessibility audit tasks.
type Auditor struct {
	db  *gorm.DB
	llm llmAuditor
}

// NewAuditor creates a new Auditor service.
func NewAuditor(db *gorm.DB, ollama *api.Client, model string) *Auditor {
	return &Auditor{
		db: db,
		llm: &ollamaAuditor{
			client: ollama,
			model:  model,
			engine: &a11y.Engine{},
		},
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
				slog.Error("Audit error", "error", err)
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

	slog.Info("Processing audit task", "profile_id", profile.ID)

	result, err := a.llm.audit(ctx, &profile)
	if err != nil {
		slog.Error("LLM audit failed", "profile_id", profile.ID, "error", err)
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
			slog.Warn("Discarding audit result due to data version mismatch", "profile_id", profile.ID)
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

	slog.Info("Audit task completed", "profile_id", profile.ID, "has_conflict", result.HasConflict)
	return true, nil
}

// audit runs the accessibility audit against the Ollama LLM.
func (o *ollamaAuditor) audit(ctx context.Context, profile *models.AccessibilityProfile) (*models.AuditResult, error) {
	o.engine.WithAuditFlags(profile)

	prompt := `You are an Accessibility Auditor specializing in logical consistency. 
Your task is to identify contradictions between the technical properties of a place and its stated accessibility status.

### DATA STRUCTURE REFERENCE:
- Profile: { "overall_status": "accessible" | "limited" | "inaccessible" | "unknown", "components": [...] }
- Component: { "type": "...", "overall_status": "...", "audit_flags": ["narrow width", "contains step", ...] }

### AUDIT RULES:

1. STATUS VS FLAGS (CRITICAL):
   - IF a component has 'audit_flags', THEN its 'overall_status' SHOULD NOT be "accessible".
   - IF the 'overall_status' is "accessible" despite 'audit_flags' being present, FLAG CONFLICT.

2. LOGICAL CONSISTENCY (Profile vs. Component):
   - IF any component (especially 'entrance') has 'overall_status' = "inaccessible", THEN the Profile's 'overall_status' MUST NOT be "accessible".
   - FLAG CONFLICT if a component is more restrictive than the Profile's overall status.

### OUTPUT FORMAT:
Respond ONLY with a JSON object:
{
  "has_conflict": bool,
  "reasoning": "A very simple, human-friendly explanation of the conflict. Avoid technical terms or jargon like 'technical limit' or 'audit flags'.",
  "confidence": float (0.0 to 1.0)
}`

	profileJSON, err := json.Marshal(profile)
	if err != nil {
		return nil, err
	}

	slog.Debug("Sending profile to LLM", "profile_id", profile.ID, "json_body", string(profileJSON))

	req := &api.GenerateRequest{
		Model:  o.model,
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
		Reasoning   string  `json:"reasoning"`
		Confidence  float64 `json:"confidence"`
	}

	err = o.client.Generate(ctx, req, func(resp api.GenerateResponse) error {
		return json.Unmarshal([]byte(resp.Response), &auditResponse)
	})
	if err != nil {
		return nil, err
	}

	return &models.AuditResult{
		HasConflict: auditResponse.HasConflict,
		Reasoning:   auditResponse.Reasoning,
		Confidence:  auditResponse.Confidence,
		LastAudit:   time.Now().Format(time.RFC3339),
	}, nil
}
