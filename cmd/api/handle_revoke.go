/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"context"
	"time"

	apiv1 "github.com/InWheelOrg/inwheel-api/internal/api/v1"
	"github.com/InWheelOrg/inwheel-api/internal/middleware"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

func (s *Server) RevokeKey(ctx context.Context, request apiv1.RevokeKeyRequestObject) (apiv1.RevokeKeyResponseObject, error) {
	r := requestFromCtx(ctx)
	if r == nil {
		return apiv1.RevokeKey401JSONResponse{Error: "unauthorized"}, nil
	}
	rawKey := r.Header.Get("X-API-Key")
	hash := middleware.SHA256Hex(rawKey)

	result := s.db.Model(&models.APIKey{}).
		Where("key_hash = ? AND revoked_at IS NULL", hash).
		Update("revoked_at", time.Now())

	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return apiv1.RevokeKey401JSONResponse{Error: "unauthorized"}, nil
	}

	return apiv1.RevokeKey204Response{}, nil
}
