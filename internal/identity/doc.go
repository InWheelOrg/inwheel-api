/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

// Package identity resolves whether an incoming non-OSM record refers to an
// existing OSM-anchored place in the places table, or whether no acceptable
// match exists.
//
// # The problem
//
// Wheelmap and future similar sources describe accessibility of physical
// places, identified by their own ID. To attach that accessibility data to
// the right row in the places table, the package decides which existing
// place an incoming record refers to, or that none of them do.
//
// # The algorithm
//
//  1. Candidate fetch: ask the repo for active places within 50 m whose
//     category is compatible with the incoming record's category. Category
//     compatibility is an allowlist: a cafe can match a restaurant; a cafe
//     cannot match a pharmacy. This hard filter eliminates the worst class
//     of wrong matches cheaply.
//
//  2. Score each candidate in [0, 1] from three signals:
//
//     - distance: linear falloff, 1.0 at 0 m, 0.0 at 50 m.
//     - name:     token-set Jaccard over normalized words. "Café Pascal"
//     and "PASCAL CAFE." normalize to the same tokens.
//     - address:  1.0 if street + house number match, 0.5 if street only,
//     0.0 otherwise. If either side lacks an address the
//     signal is dropped and its weight is redistributed
//     proportionally between distance and name (final score
//     still in [0, 1]).
//
//     Weighted sum: 0.5*distance + 0.4*name + 0.1*address.
//
//  3. Decide on the best candidate's score:
//
//     - >= 0.80 -> Confident       (attach to that place)
//     - >= 0.55 -> LowConfidence   (attach, but record the low score)
//     - <  0.55 -> NoMatch         (enqueue in unmatched_external)
//
// # What this package does not do
//
//   - Persist anything. Match returns a Decision; callers do the writes.
//   - Talk to any upstream source. Callers shape rows into identity.Record.
//   - Tune thresholds at runtime. Constants in score.go are reviewed in PRs
//     against a fixture set so behaviour drift is visible at review time.
package identity
