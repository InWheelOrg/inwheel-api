/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	apiv1 "github.com/InWheelOrg/inwheel-server/internal/api/v1"
	"github.com/getkin/kin-openapi/openapi3filter"
)

// validationErrorHandler converts spec-validation errors into the same
// {error, fields} JSON shape that internal/validation produces. It is wired as:
//   - ErrorHandlerFunc on StdHTTPServerOptions (param-parsing errors from generated wrapper)
//   - ErrorHandlerFunc on OapiRequestValidatorWithOptions (kin-openapi middleware errors)
func (s *Server) validationErrorHandler(w http.ResponseWriter, _ *http.Request, err error) {
	// Security/auth errors
	var secErr *openapi3filter.SecurityRequirementsError
	if errors.As(err, &secErr) {
		for _, inner := range secErr.Errors {
			if errors.Is(inner, errRateLimited) {
				w.Header().Set("Retry-After", strconv.Itoa(s.keyLimiter.RetryAfterSeconds()))
				writeJSON(w, map[string]string{"error": "rate limit exceeded"}, http.StatusTooManyRequests)
				return
			}
		}
		writeJSON(w, map[string]string{"error": "unauthorized"}, http.StatusUnauthorized)
		return
	}

	field, reason := extractFieldError(err)

	type fieldError struct {
		Field  string `json:"field"`
		Reason string `json:"reason"`
	}
	body := struct {
		Error  string       `json:"error"`
		Fields []fieldError `json:"fields"`
	}{
		Error:  "validation failed",
		Fields: []fieldError{{Field: field, Reason: reason}},
	}

	payload, marshalErr := json.Marshal(body)
	if marshalErr != nil {
		slog.Error("validationErrorHandler: marshal failed", "error", marshalErr)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	if _, err := w.Write(payload); err != nil {
		slog.Error("validationErrorHandler: write failed", "error", err)
	}
}

// extractFieldError pulls a field name and human-readable reason out of a
// spec-validation error from either kin-openapi or the oapi-codegen wrapper.
func extractFieldError(err error) (field, reason string) {
	// kin-openapi middleware errors.
	var reqErr *openapi3filter.RequestError
	if errors.As(err, &reqErr) {
		if reqErr.Parameter != nil {
			return reqErr.Parameter.Name, reqErr.Reason
		}
		if reqErr.RequestBody != nil {
			return "body", reqErr.Reason
		}
		return "request", reqErr.Reason
	}

	// oapi-codegen generated wrapper errors
	var ipfErr *apiv1.InvalidParamFormatError
	if errors.As(err, &ipfErr) {
		return ipfErr.ParamName, ipfErr.Err.Error()
	}
	var rpErr *apiv1.RequiredParamError
	if errors.As(err, &rpErr) {
		return rpErr.ParamName, "is required"
	}
	var umpErr *apiv1.UnmarshalingParamError
	if errors.As(err, &umpErr) {
		return umpErr.ParamName, "invalid format"
	}
	var tmvErr *apiv1.TooManyValuesForParamError
	if errors.As(err, &tmvErr) {
		return tmvErr.ParamName, "must have a single value"
	}

	// Body decode errors from the generated strict handler have the form
	// "can't decode JSON body: field: reason". Unwrap and parse "field: reason".
	if inner := errors.Unwrap(err); inner != nil {
		msg := inner.Error()
		if idx := strings.Index(msg, ": "); idx > 0 {
			return msg[:idx], msg[idx+2:]
		}
		return "body", msg
	}

	return "request", err.Error()
}
