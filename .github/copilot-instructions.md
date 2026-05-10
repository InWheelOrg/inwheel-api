# Copilot Instructions

## Commands

```bash
# Build
go build ./...

# Unit tests
go test ./...

# Single package tests
go test ./internal/a11y/...
go test ./internal/validation/...

# Single test by name
go test ./internal/a11y/... -run TestDetectConflicts

# Integration tests (requires Docker)
go test -tags integration -timeout 120s ./...

# Lint (uses .golangci.yml config)
golangci-lint run ./...

# Regenerate API server code from OpenAPI spec
go generate ./internal/api/v1/...

# Start full stack
cp .env.example .env   # first time only
docker compose up
```

## Architecture

REST API for storing and retrieving accessibility data about physical places. The API is a **pure data layer** — it stores accessibility facts but never computes whether a place is "accessible". Clients apply their own relevance logic per user profile.

### Data flow

1. Client submits accessibility data via API
2. `internal/validation` runs structural checks → HTTP 400 with `[]FieldError` on failure
3. `internal/a11y.Engine` computes `AuditFlags` from component properties, then checks for conflicts between submitted `OverallStatus` and those flags
4. Conflicts → HTTP 422 with contradictions list; caller must reconcile
5. Clean data is persisted as-is

### Code generation

The API server code is generated from `api/openapi.yaml` using `oapi-codegen`. Handlers in `cmd/api/` implement the generated strict server interface from `internal/api/v1/server.gen.go`. CI verifies generated code is up to date — always run `go generate ./internal/api/v1/...` after changing the OpenAPI spec.

### Key packages

| Package | Purpose |
|---|---|
| `cmd/api` | REST API binary — handlers, error formatting, route wiring |
| `pkg/models` | Domain types shared across services (`Place`, `AccessibilityProfile`, `A11yComponent`) |
| `internal/a11y` | Stateless rule engine: `WithAuditFlags()`, `DetectConflicts()`, `ComputeEffectiveProfile()` |
| `internal/validation` | Structural request validation — pure functions returning `[]FieldError` |
| `internal/geo` | PostGIS spatial queries (`ST_DWithin`, `ST_MakeEnvelope`) |
| `internal/db` | GORM setup, embedded SQL migrations via `golang-migrate` |
| `internal/pagination` | Cursor-based pagination — base64url-encoded `RFC3339Nano|uuid` |
| `internal/audit` | Append-only write log; failures warn but never block writes |
| `internal/middleware` | Rate limiting (per-key token buckets), request logging with `X-Request-ID`, API key context |

## Conventions

### HTTP and routing

- Uses stdlib `http.ServeMux` with Go 1.22+ path parameters (`/places/{id}`), no HTTP framework
- Routes: unversioned health endpoints (`/healthz`, `/readyz`, `/openapi.yaml`) on outer mux; `/v1/*` on inner mux with OpenAPI request validation middleware
- Error responses are JSON: `{"error":"...", "fields":[{"field":"...","reason":"..."}]}` for 400s; `{"error":"..."}` for other errors

### Validation

- `internal/validation` handles constraints OpenAPI can't express (blank strings, tag limits, query-group exclusivity, cursor format)
- Validation functions return `[]FieldError`; callers check `len(errs) > 0`
- Runs **before** the a11y engine; 400 = structural validation, 422 = semantic conflicts

### Models and JSONB

- `Geometry`, `PlaceTags`, `ExternalIDs`, and `A11yComponents` implement `driver.Valuer` / `sql.Scanner` for PostgreSQL JSONB columns
- `AuditFlags` are deterministic facts computed from property values (e.g., entrance width < 0.8m → flag), never judgments about accessibility
- `OverallStatus` on a component is client-submitted; the engine only rejects it when directly contradicted by hard flags

### Database

- GORM with PostgreSQL + PostGIS
- Migrations are embedded SQL files in `internal/db/migrations/` using `golang-migrate`
- `Place` supports parent-child hierarchy (malls, airports) via optional `ParentID`

### Testing

- Table-driven tests throughout: `tests := []struct{...}` + `for _, tt := range tests { t.Run(...) }`
- HTTP tests use `httptest.NewRequest` / `httptest.NewRecorder`
- Integration tests use `testcontainers-go` to spin up real PostgreSQL/PostGIS — requires Docker, build tag `integration`
- Test helpers in `internal/testhelpers/`: `StartPostgres(...)` returns a connected GORM DB and cleanup function
