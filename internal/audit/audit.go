/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

// Package audit provides a lightweight append-only write log for tracking
// which API key created or modified each record. Failures are logged as
// warnings but never propagated — audit must not block application writes.
package audit

import (
	"log/slog"
	"time"

	"gorm.io/gorm"
)

// WriteLog is a single row in the write_logs table. It records which API key
// performed a create or update on a given record.
type WriteLog struct {
	ID          string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	TargetTable string    `gorm:"column:target_table;not null"`
	RecordID    string    `gorm:"type:uuid;not null;index"`
	APIKeyID    *string   `gorm:"type:uuid;index"`
	Action      string    `gorm:"not null"`
	CreatedAt   time.Time
}

// Log appends a row to write_logs. apiKeyID may be empty (stored as NULL).
func Log(db *gorm.DB, targetTable, recordID, apiKeyID, action string) {
	entry := WriteLog{
		TargetTable: targetTable,
		RecordID:    recordID,
		Action:      action,
	}
	if apiKeyID != "" {
		entry.APIKeyID = &apiKeyID
	}
	if err := db.Create(&entry).Error; err != nil {
		slog.Warn("audit log write failed",
			"table", targetTable,
			"record_id", recordID,
			"action", action,
			"error", err,
		)
	}
}
