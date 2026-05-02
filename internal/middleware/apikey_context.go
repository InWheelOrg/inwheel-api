/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package middleware

import "context"

type ctxKeyAPIKeyID struct{}

// WithAPIKeyID returns a new context carrying the authenticated API key's DB UUID.
func WithAPIKeyID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyAPIKeyID{}, id)
}

// APIKeyIDFromCtx returns the API key DB UUID stored by WithAPIKeyID, or "" if absent.
func APIKeyIDFromCtx(ctx context.Context) string {
	id, _ := ctx.Value(ctxKeyAPIKeyID{}).(string)
	return id
}
