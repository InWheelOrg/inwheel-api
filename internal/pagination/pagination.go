/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

// Package pagination provides cursor-based pagination helpers for the InWheel API.
// Cursors are base64url-encoded but not signed — do not reuse on endpoints where
// cursor integrity or per-user isolation matters.
package pagination

import (
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Page is the standard paginated response envelope returned by list endpoints.
type Page[T any] struct {
	Data       []T    `json:"data"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// Encode produces a base64url-encoded cursor from a timestamp and UUID.
// Truncates to µs to match PostgreSQL timestamptz precision.
func Encode(t time.Time, id string) string {
	raw := t.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// Decode parses a cursor produced by Encode and returns its timestamp and UUID components.
func Decode(cursor string) (time.Time, string, error) {
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", errors.New("invalid cursor encoding")
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", errors.New("invalid cursor format")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", errors.New("invalid cursor timestamp")
	}
	if _, err := uuid.Parse(parts[1]); err != nil {
		return time.Time{}, "", errors.New("invalid cursor id")
	}
	return t, parts[1], nil
}
