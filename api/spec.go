/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

// Package api exposes the raw OpenAPI spec file for embedding.
package api

import _ "embed"

//go:embed openapi.yaml
var SpecYAML []byte
