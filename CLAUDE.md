# CLAUDE.md

Public REST API for InWheel — a global registry of physical-place accessibility data. Pure data layer: stores and returns accessibility facts, never decides whether a place is accessible for a given user.

Two binaries share one PostgreSQL+PostGIS database and the domain types in `pkg/models`:

- **`cmd/api`** — REST API on port 8080. OpenAPI-driven via oapi-codegen strict server. Owns place reads, place creation, accessibility profile writes, API-key issuance/revocation. Every write runs through structural validation (`internal/validation`) then the a11y rule engine (`internal/a11y`), which computes audit flags from component properties and rejects hard self-contradictions (e.g. `OverallStatus=accessible` with `step + no ramp`).
- **`cmd/ingestion`** — one-shot CLI. Two pipelines selected by `sources.SourceKind`:
  - **Canonical** (OSM today): streams `(models.Place, *models.AccessibilityProfile)` → batcher (size 1000) → `place.UpsertBatch` on `(osm_id, osm_type)` → batcher accumulates `RETURNING id` values → `place.UpsertProfileIngestion` per non-nil profile (skips `user_verified=true` rows) → `identity.Sweeper` drains `unmatched_external` rows near the touched places.
  - **External** (Wheelmap and similar): streams `identity.Record` → `identity.Resolver` → Confident/LowConfidence attaches `ExternalRef` via `jsonb_set`; NoMatch enqueues to `unmatched_external` with full matchable signal (name/category/street/housenumber/lat/lng).

## Key packages

| Package | Role |
|---|---|
| `pkg/models` | Shared domain types (`Place`, `AccessibilityProfile`, `A11yComponent`, `ExternalRef`, `UnmatchedExternal`, `APIKey`). JSONB customs (`Geometry`, `PlaceTags`, `A11yComponents`, `ExternalIDs`) implement `driver.Valuer`/`sql.Scanner`. |
| `internal/a11y` | Audit-flag computation, hard-conflict detection, `ComputeEffectiveProfile` for parent inheritance. |
| `internal/identity` | Match algorithm (50 m radius, category allowlist, weighted score 0.5/0.4/0.1 across distance/name/address, thresholds 0.80 Confident / 0.55 LowConfidence). `Resolver` is the at-write driver; `Sweeper` is the post-canonical-ingest retry driver. Package itself is I/O-free. |
| `internal/sources` + `internal/sources/osm` | Capability interfaces and OSM concrete source (tag allowlist, transformer, PBF streamer). |
| `internal/place` | `places` and `accessibility_profiles` repo. `UpsertBatch`, `AttachExternalRef`, `FindCandidates`, `UpsertProfile` (API write path), `UpsertProfileIngestion` (ingestion write path, skips `user_verified`). Satisfies `identity.CandidateRepo`/`AttachRepo`. |
| `internal/unmatched` | `unmatched_external` queue repo. `Enqueue`, `FindCandidatesNearTouched` (set-based `ST_DWithin` join), `BumpAttempts`, `Delete`. Satisfies `identity.EnqueueRepo`/`SweepRepo`. |
| `internal/validation` | Pure structural validation over `pkg/models` types. Entrypoints: `Place`, `PlacesQuery`, `Email`. |
| `internal/db` | GORM + numbered SQL migrations + PostGIS index setup. |
| `internal/audit` | Append-only `write_logs` of `(target_table, record_id, api_key_id, action)`. |
| `internal/middleware` | Request logging, rate limiter, API-key context plumbing. |
| `internal/api/v1` | Generated from `api/openapi.yaml` by oapi-codegen. Do not edit. |

Per-package READMEs cover detail. Auth is `X-API-Key` (SHA-256 hashed at rest), 60 req/s per key, 3 registrations per 20 min per IP. Integration tests use `testcontainers-go` for real PostGIS, build tag `integration`.
