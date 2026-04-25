# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run unit tests
go test ./...

# Run integration tests (requires Docker)
go test -tags integration -timeout 120s ./...

# Lint
go vet ./...

# Start full stack (PostgreSQL + PostGIS, API)
cp .env.example .env   # first time only
docker compose up
```

## Architecture

Two active binaries under `cmd/`:

| Service | Port | Role |
|---|---|---|
| `cmd/api` | 8080 | REST API — reads/writes places and accessibility data |
| `cmd/ingestion` | — | Placeholder — future OpenStreetMap sync |

**Data flow:**
1. Client submits accessibility data via API
2. `a11y.Engine` runs synchronously: computes `AuditFlags` from component properties, then checks for conflicts between submitted `OverallStatus` and those flags
3. Conflicts return HTTP 422 with a list of contradictions — caller must reconcile
4. Clean data is persisted as-is

The API is a pure data layer. It stores and returns accessibility facts; it never computes whether a place is "accessible". Clients apply their own relevance logic per user profile.

## Key Packages

**`pkg/models`** — Domain types shared across services:
- `Place`: Location with optional parent (hierarchy for malls/airports)
- `AccessibilityProfile`: `OverallStatus` (client-submitted, server-validated) + array of `A11yComponent`
- `A11yComponent`: Typed feature (entrance, restroom, parking, elevator) with `AuditFlags` computed on every write from property values

**`internal/a11y`** — Accessibility rule engine:
- `Engine.WithAuditFlags()` — populates `AuditFlags` on each component deterministically from property values (e.g. entrance width < 0.8m → `"narrow width (0.8m required)"`)
- `Engine.DetectConflicts()` — returns conflicts where a component is marked `StatusAccessible` but carries technical violation flags; called after `WithAuditFlags`
- `Engine.ComputeEffectiveProfile()` — resolves inherited components from parent places

**`internal/geo`** — PostGIS spatial queries (`ST_DWithin`, `ST_MakeEnvelope`)

**`internal/db`** — GORM setup, AutoMigrate, PostGIS spatial index creation

## Patterns

- **No HTTP framework** — uses stdlib `http.ServeMux` with Go 1.22+ path parameters (`/places/{id}`)
- **JSONB custom types** — `Geometry`, `PlaceTags`, and component `Properties` implement `driver.Valuer` / `sql.Scanner`
- **AuditFlags are facts, not judgments** — a narrow entrance is a stored fact computed from width; the API never decides if that makes a place inaccessible
- **Component status is client-submitted** — `OverallStatus` on a component is declared by the submitter; the engine only rejects it when directly contradicted by flags

## Testing Notes

- Integration tests use `testcontainers-go` to spin up a real PostgreSQL/PostGIS instance — Docker must be running
- Build tag: `integration`
- Test helpers live in `internal/testhelpers/`
