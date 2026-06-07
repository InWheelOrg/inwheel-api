# internal/sources

A tiny interface package that defines what makes something an ingestible source for `cmd/ingestion`. It contains no logic — just type definitions. Concrete sources live under `internal/sources/<name>/`. The package lives in `internal/` because no external module should implement these interfaces; the dispatcher in `cmd/ingestion` type-asserts against the capability interfaces below.

## The discriminator: `SourceKind`

Every `Source` declares whether it is canonical or external. The dispatcher uses this to pick the pipeline.

| Kind | Meaning | Today |
|---|---|---|
| `SourceKindCanonical` | The source owns place rows. Emits `models.Place`. | OSM |
| `SourceKindExternal` | The source contributes accessibility data and external IDs that attach to existing canonical places via `identity.Match`. Emits `identity.Record`. | (none yet — Wheelmap is the planned first) |

## Sinks

The pipeline gives each source a sink to write into rather than letting the source touch the DB directly. The sink type is chosen by `SourceKind`:

| Sink | Receives | Used by |
|---|---|---|
| `Sink` | `models.Place` | canonical sources |
| `RecordSink` | `identity.Record` | external sources |

## Capability interfaces

A source implements only the capability interfaces it actually supports. The dispatcher does a type assertion at runtime and returns a clear error if a command is requested but the matching interface is missing.

| Interface | Sink kind | Operation |
|---|---|---|
| `FullImporter` | `Sink` | Full pull from a canonical source |
| `DiffSyncer` | `Sink` | Incremental pull since a timestamp (canonical) |
| `ExternalFullImporter` | `RecordSink` | Full pull from an external source |
| `ExternalDiffSyncer` | `RecordSink` | Incremental pull since a timestamp (external) |

The split mirrors the two pipelines in `cmd/ingestion`. A new source provides whichever subset it can support — full-import is the minimum to be useful; diff-sync is an optimisation for ongoing sync.

## Why import `internal/identity`?

`RecordSink` takes `identity.Record`, the shared shape for an incoming external record. This is the only legitimate cross-internal import in the package and keeps external sources from having to redefine the same struct.

## Adding a new source

1. Create `internal/sources/<name>/`.
2. Implement `Source` (`Name`, `Kind`) plus one or more capability interfaces.
3. Register the source in `cmd/ingestion/registry.go` — a single `case` in `buildSource`.
