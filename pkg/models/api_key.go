/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package models

import "time"

// APIKey represents a registered API credential. Only the SHA-256 hash of the
// raw key is persisted; the raw key is returned to the caller once at registration.
type APIKey struct {
	ID        string     `json:"id"         gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	Email     string     `json:"email"      gorm:"not null;index"`
	KeyHash   string     `json:"-"          gorm:"uniqueIndex;not null"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}
