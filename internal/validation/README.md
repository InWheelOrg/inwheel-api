# internal/validation

Pure functions over `pkg/models` types that enforce constraints the OpenAPI spec cannot express. Returns `[]FieldError`; handlers convert a non-empty slice to an HTTP 400 response with a `{error, fields[]}` JSON body. This package has no side effects and no dependencies on the DB or the a11y engine.

## Division of responsibility

Two validation layers run on every mutating request:

- **OpenAPI middleware** (kin-openapi, runs at the edge) — required fields, type coercion, enum membership, numeric bounds defined in the spec, UUID format. By the time a handler runs, all spec-level constraints are already enforced.
- **This package** (runs inside the handler) — constraints that require cross-field logic or are not expressible in JSON Schema: whitespace-only strings, mutually exclusive query-param groups, bounding-box coordinate ordering, cursor decode validity, tag size limits.

The division keeps the spec the source of truth for the interface contract while keeping multi-field business validity in tested Go code.

## Entrypoint functions

| Function | Validates |
|---|---|
| `Email(s string)` | Regex check for a syntactically valid email address (used during key registration) |
| `Place(p *models.Place)` | Non-blank name, tag entry count (≤ 50), tag key/value length limits |
| `PlacesQuery(p PlacesQueryParams)` | Proximity params (`lng`/`lat`/`radius`) must all be present or all absent; bbox params (`min_lng`/`min_lat`/`max_lng`/`max_lat`) same; the two groups are mutually exclusive; min < max for bbox; cursor decodes without error |

`AccessibilityProfile` has no validation entrypoint in this package. `Place()` does not inspect `p.Accessibility` at all — inline accessibility data on `POST /places` gets no structural check beyond the OpenAPI spec. `PATCH /places/{id}/accessibility` doesn't call into this package either; it relies solely on spec-level validation before `internal/a11y` computes audit flags and the write persists.

`PlacesQueryParams` mirrors the oapi-codegen-generated `ListPlacesParams` but lives in this package to avoid an upward import dependency on `internal/api/v1`.

## Size limits

| Field | Limit |
|---|---|
| Name | 256 characters (spec-enforced; whitespace-only additionally rejected here) |
| Tags entries | 50 |
| Tag key | 64 characters |
| Tag value | 256 characters |
| Parking count | 10 000 (spec-enforced) |
