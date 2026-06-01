# Review instructions

## What Important means here

Reserve 🔴 Important for findings that would break behavior, leak data, corrupt the
registry, or violate a core architectural invariant: incorrect logic, SQL injection or
unparameterised queries, PII in logs, missing auth on write endpoints, panics on
untrusted input, and violations of the invariants below. Style, naming, and refactoring
suggestions are 🟡 Nit at most.

## Core invariants — always check, always Important

- **The API is a pure data layer.** It stores and returns accessibility facts; it
  never decides whether a place is "accessible" for a given user. Flag any handler,
  service, or query that returns a boolean/score representing aggregated accessibility
  judgment as 🔴 Important.
- **`AuditFlags` are deterministic facts computed from component properties** by
  `internal/a11y` on every write. Flag any code path that writes `AuditFlags` from
  client input, or computes them non-deterministically (clock, random, network), as
  🔴 Important.
- **Validation runs before the a11y engine.** Structural checks (`internal/validation`)
  must precede semantic conflict detection (`internal/a11y.DetectConflicts`). Flag
  reversed ordering as 🔴 Important.
- **No HTTP framework.** Handlers use stdlib `net/http` with `http.ServeMux` and Go
  1.22+ path parameters. Flag introduction of gin / chi / echo / fiber as 🔴 Important.

## Cap the nits

Report at most five 🟡 Nits per review. If you found more, say "plus N similar items"
in the summary. If all findings are nits, lead the summary with "No blocking issues."

## Do not report

- Formatting, import ordering, or lint issues — `go vet` and CI handle these
- Missing tests for code paths that cannot fail or are already covered
- Documentation suggestions unless CLAUDE.md explicitly requires docs for this case
- Hypothetical future concerns without concrete evidence in the diff

## Always check

- All SQL / GORM queries use parameterised placeholders, never string concatenation
  of user input
- JSONB custom types (`Geometry`, `PlaceTags`, component `Properties`) implement both
  `driver.Valuer` and `sql.Scanner` symmetrically
- New write endpoints run validation → a11y engine → persist, in that order
- Errors from `internal/a11y.DetectConflicts` return HTTP 422 (not 400 or 500)
- Structural validation failures return HTTP 400 with a field-level error list
- No accessibility "scoring" or "decision" logic leaks into handlers or services
