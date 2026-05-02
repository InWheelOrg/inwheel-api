/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"

	apiv1 "github.com/InWheelOrg/inwheel-server/internal/api/v1"
	"github.com/InWheelOrg/inwheel-server/internal/middleware"
	"github.com/InWheelOrg/inwheel-server/internal/validation"
	"github.com/InWheelOrg/inwheel-server/pkg/models"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const pgUniqueViolation = "23505"

func (s *Server) Register(ctx context.Context, request apiv1.RegisterRequestObject) (apiv1.RegisterResponseObject, error) {
	if r := requestFromCtx(ctx); r != nil {
		ip := middleware.ClientIP(r)
		if !s.regLimiter.Allow(ip) {
			return apiv1.Register429JSONResponse{Error: "rate limit exceeded"}, nil
		}
	}

	email := string(request.Body.Email)

	if errs := validation.Email(email); len(errs) > 0 {
		return apiv1.Register400JSONResponse(validationError(errs)), nil
	}

	var existing models.APIKey
	if err := s.db.Where("email = ? AND revoked_at IS NULL", email).First(&existing).Error; err == nil {
		return apiv1.Register409JSONResponse{Error: "an active key already exists for this email"}, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		slog.Error("registration: lookup failed", "error", err)
		return nil, err
	}

	rawKey, err := generateKey()
	if err != nil {
		slog.Error("registration: key generation failed", "error", err)
		return nil, err
	}

	record := models.APIKey{Email: email, KeyHash: middleware.SHA256Hex(rawKey)}
	if err := s.db.Create(&record).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return apiv1.Register409JSONResponse{Error: "an active key already exists for this email"}, nil
		}
		slog.Error("registration: insert failed", "error", err)
		return nil, err
	}

	return apiv1.Register201JSONResponse{
		ApiKey:    rawKey,
		CreatedAt: record.CreatedAt,
		Email:     record.Email,
	}, nil
}

func generateKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "iwk_" + hex.EncodeToString(b), nil
}
