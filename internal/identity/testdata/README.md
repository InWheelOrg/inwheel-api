# identity matcher fixtures

`match_fixtures.json` is the source-agnostic regression set for `identity.Match`. Each entry pairs an incoming `identity.Record` with a list of candidate places and the decision the matcher must produce. `TestMatch_Fixtures` in `../matcher_test.go` runs every entry and asserts the result.

The goal is to lock in current matcher behaviour and catch accidental regressions when scoring, normalization, blocking, or thresholds are touched. This file is **not** a tuning corpus — it does not measure aggregate precision/recall against realistic data. Per-source realism fixtures live under `internal/sources/<name>/testdata/` and arrive with each new source.

## Entry format

```json
{
  "name": "short label that appears as the subtest name",
  "record": { "Name": "...", "Lat": 0.0, "Lng": 0.0, "Category": "...",
              "Street": "...", "HouseNumber": "..." },
  "candidates": [
    { "id": "p1", "name": "...", "lat": 0.0, "lng": 0.0, "category": "...",
      "tags": { "addr:street": "...", "addr:housenumber": "..." } }
  ],
  "expected": { "Kind": "confident|low_confidence|no_match", "MatchedPlaceID": "p1" }
}
```

- `record` fields mirror `identity.Record`. Omitted string fields default to `""`.
- `candidates` are `models.Place` values. The fake repo in the test applies the category compat filter (`identity.Compatible`) before passing them to `Match`, so entries can include incompatible distractors and verify they are excluded.
- `expected.Kind` is required. `expected.MatchedPlaceID` is asserted only when non-empty; omit it for `no_match` entries.
- Confidence values are **not** asserted. Specific scores shift whenever a weight or threshold changes; only the decision band matters for a regression test.

## Adding a new entry

1. Pick what behaviour the entry pins down — name normalization, distance falloff, compat filter, threshold band, tiebreak, etc. One concern per entry.
2. Compute the expected outcome by hand from the constants in `score.go` (`RadiusM`, `ConfidentThreshold`, `LowConfidenceThreshold`, the three weights). If the outcome depends on tuning being exactly what it is today, that is a signal the entry will be noisy and may need to be rewritten when thresholds change.
3. Keep coordinates in the same neighbourhood as existing entries (around `(46.4628, 6.8417)`). Compute lat offsets as `meters / 111000`; longitude offsets are not used by the current set.
4. Run `go test ./internal/identity/ -run TestMatch_Fixtures -v` and confirm the new entry passes.

## What is covered today

- Confident, low-confidence, and no-match outcomes across coordinate, name, and address signals.
- Distance falloff at 5 m, 25 m, 35 m, 49 m, and beyond `RadiusM`.
- Name normalization: diacritics, business-suffix drop, word reorder, partial overlap.
- Category compat: candidates of an incompatible category are filtered out by blocking.
- Tiebreak: stronger name beats slightly-closer distractor; argmax across multiple candidates.
- Address weight: matching street + housenumber boosts the score; mismatched address does not block a confident match driven by name and distance.
