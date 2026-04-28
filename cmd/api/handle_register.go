/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/InWheelOrg/inwheel-server/internal/middleware"
	"github.com/InWheelOrg/inwheel-server/internal/validation"
	"github.com/InWheelOrg/inwheel-server/pkg/models"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// pgUniqueViolation is the Postgres SQLSTATE for a unique-constraint violation.
const pgUniqueViolation = "23505"

// handleRegister handles POST /auth/register.
// It issues a new API key for the given email address. The raw key is returned
// once and never stored — only its SHA-256 hash is persisted.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	ip := middleware.ClientIP(r)
	if !s.regLimiter.Allow(ip) {
		w.Header().Set("Retry-After", strconv.Itoa(s.regLimiter.RetryAfterSeconds()))
		jsonResponse(w, map[string]string{"error": "rate limit exceeded"}, http.StatusTooManyRequests)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, map[string]string{"error": "invalid request body"}, http.StatusBadRequest)
		return
	}

	if errs := validation.Email(req.Email); len(errs) > 0 {
		jsonResponse(w, validationError(errs), http.StatusBadRequest)
		return
	}

	var existing models.APIKey
	err := s.db.Where("email = ? AND revoked_at IS NULL", req.Email).First(&existing).Error
	if err == nil {
		jsonResponse(w, map[string]string{"error": "an active key already exists for this email"}, http.StatusConflict)
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		slog.Error("registration: lookup failed", "error", err)
		jsonResponse(w, map[string]string{"error": "internal server error"}, http.StatusInternalServerError)
		return
	}

	rawKey, err := generateKey()
	if err != nil {
		slog.Error("registration: key generation failed", "error", err)
		jsonResponse(w, map[string]string{"error": "internal server error"}, http.StatusInternalServerError)
		return
	}

	record := models.APIKey{Email: req.Email, KeyHash: middleware.SHA256Hex(rawKey)}
	if err := s.db.Create(&record).Error; err != nil {
		// Concurrent registration that lost the partial-unique-index race: surface as 409.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			jsonResponse(w, map[string]string{"error": "an active key already exists for this email"}, http.StatusConflict)
			return
		}
		slog.Error("registration: insert failed", "error", err)
		jsonResponse(w, map[string]string{"error": "internal server error"}, http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]string{
		"key":  rawKey,
		"note": "Save this key — it will not be shown again.",
	}, http.StatusCreated)
}

// generateKey produces a key with the "iwk_" prefix followed by 64 hex characters (32 random bytes).
func generateKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "iwk_" + hex.EncodeToString(b), nil
}
