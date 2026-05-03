/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/InWheelOrg/inwheel-api/internal/middleware"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
	"github.com/getkin/kin-openapi/openapi3filter"
	"gorm.io/gorm"
)

var (
	errUnauthorized = errors.New("unauthorized")
	errRateLimited  = errors.New("rate limit exceeded")
)

func (s *Server) authenticate(ctx context.Context, ai *openapi3filter.AuthenticationInput) error {
	if ai.SecuritySchemeName != "ApiKeyAuth" {
		return fmt.Errorf("unsupported security scheme %q", ai.SecuritySchemeName)
	}
	r := ai.RequestValidationInput.Request
	rawKey := r.Header.Get("X-API-Key")
	if rawKey == "" {
		return errUnauthorized
	}
	hash := middleware.SHA256Hex(rawKey)
	if !s.keyLimiter.Allow(hash) {
		return errRateLimited
	}
	var apiKey models.APIKey
	if err := s.db.Where("key_hash = ? AND revoked_at IS NULL", hash).First(&apiKey).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errUnauthorized
		}
		return err
	}

	// Enrich the request context with the API key ID so downstream handlers and
	// the request logger can access it without an additional DB lookup.
	req := ai.RequestValidationInput.Request
	enriched := middleware.WithAPIKeyID(req.Context(), apiKey.ID)
	middleware.SetLogAPIKeyID(enriched, apiKey.ID)
	*req = *req.WithContext(enriched)

	return nil
}
