/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

type ctxKeyRequestID struct{}
type ctxKeyLogFields struct{}

// logFields is a mutable carrier placed in context by RequestLogger so that
// later middleware (e.g. auth) can enrich the log line after the fact.
type logFields struct {
	mu       sync.Mutex
	apiKeyID string
}

func (f *logFields) setAPIKeyID(id string) {
	f.mu.Lock()
	f.apiKeyID = id
	f.mu.Unlock()
}

func (f *logFields) getAPIKeyID() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.apiKeyID
}

// RequestLogger logs a structured JSON line for every HTTP request on completion.
// Fields: method, path, status, latency_ms, ip, request_id, api_key_id.
// Log level: Info for 1xx–3xx, Warn for 4xx, Error for 5xx.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := uuid.New().String()

		fields := &logFields{}
		ctx := context.WithValue(r.Context(), ctxKeyRequestID{}, reqID)
		ctx = context.WithValue(ctx, ctxKeyLogFields{}, fields)
		r = r.WithContext(ctx)

		w.Header().Set("X-Request-ID", reqID)

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"latency_ms", time.Since(start).Milliseconds(),
			"ip", ClientIP(r),
			"request_id", reqID,
			"api_key_id", fields.getAPIKeyID(),
		}
		switch {
		case rec.status >= 500:
			slog.Error("http request", attrs...)
		case rec.status >= 400:
			slog.Warn("http request", attrs...)
		default:
			slog.Info("http request", attrs...)
		}
	})
}

// RequestIDFromCtx returns the request ID stored by RequestLogger, or "" if absent.
func RequestIDFromCtx(ctx context.Context) string {
	id, _ := ctx.Value(ctxKeyRequestID{}).(string)
	return id
}

// SetLogAPIKeyID writes the API key DB ID into the mutable log carrier for this
// request. Call this from the auth layer after resolving the key; the logger
// reads it when writing the final log line.
func SetLogAPIKeyID(ctx context.Context, id string) {
	if f, ok := ctx.Value(ctxKeyLogFields{}).(*logFields); ok {
		f.setAPIKeyID(id)
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
