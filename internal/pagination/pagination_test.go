/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package pagination

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	ts := time.Date(2026, 5, 1, 12, 0, 0, 123456000, time.UTC)
	id := "11111111-2222-3333-4444-555555555555"

	cursor := Encode(ts, id)

	gotTS, gotID, err := Decode(cursor)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if !gotTS.Equal(ts) {
		t.Errorf("timestamp = %v, want %v", gotTS, ts)
	}
	if gotID != id {
		t.Errorf("id = %q, want %q", gotID, id)
	}
}

func TestDecodeInvalidBase64(t *testing.T) {
	_, _, err := Decode("not!valid!base64!!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestDecodeNoSeparator(t *testing.T) {
	raw := base64.RawURLEncoding.EncodeToString([]byte("2026-05-01T12:00:00Z"))
	_, _, err := Decode(raw)
	if err == nil {
		t.Error("expected error when separator is missing")
	}
}

func TestDecodeInvalidTimestamp(t *testing.T) {
	raw := base64.RawURLEncoding.EncodeToString([]byte("not-a-timestamp|11111111-2222-3333-4444-555555555555"))
	_, _, err := Decode(raw)
	if err == nil {
		t.Error("expected error for invalid timestamp")
	}
}

func TestDecodeInvalidUUID(t *testing.T) {
	raw := base64.RawURLEncoding.EncodeToString([]byte("2026-05-01T12:00:00Z|not-a-uuid"))
	_, _, err := Decode(raw)
	if err == nil {
		t.Error("expected error for invalid uuid")
	}
}

func TestEncodeIsAlwaysUTC(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Helsinki")
	ts := time.Date(2026, 5, 1, 15, 0, 0, 0, loc) // UTC+3

	cursor := Encode(ts, "11111111-2222-3333-4444-555555555555")
	gotTS, _, _ := Decode(cursor)

	if gotTS.Location() != time.UTC {
		t.Errorf("decoded timestamp location = %v, want UTC", gotTS.Location())
	}
	if !gotTS.Equal(ts) {
		t.Errorf("decoded timestamp = %v, want %v (in UTC)", gotTS, ts.UTC())
	}
}
