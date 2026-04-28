/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/InWheelOrg/inwheel-server/internal/middleware"
	"github.com/InWheelOrg/inwheel-server/pkg/models"
	"gorm.io/gorm"
)

// handleRevokeKey handles DELETE /auth/keys.
// The bearer token in the Authorization header identifies which key to revoke.
func (s *Server) handleRevokeKey(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		jsonResponse(w, map[string]string{"error": "unauthorized"}, http.StatusUnauthorized)
		return
	}

	hash := middleware.SHA256Hex(strings.TrimPrefix(authHeader, "Bearer "))

	var apiKey models.APIKey
	err := s.db.Where("key_hash = ? AND revoked_at IS NULL", hash).First(&apiKey).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			jsonResponse(w, map[string]string{"error": "unauthorized"}, http.StatusUnauthorized)
			return
		}
		slog.Error("revoke: key lookup failed", "error", err)
		jsonResponse(w, map[string]string{"error": "internal server error"}, http.StatusInternalServerError)
		return
	}

	if err := s.db.Model(&apiKey).Update("revoked_at", time.Now()).Error; err != nil {
		slog.Error("revoke: update failed", "error", err)
		jsonResponse(w, map[string]string{"error": "internal server error"}, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
