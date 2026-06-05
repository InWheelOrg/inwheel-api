/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package identity

// businessSuffixes are tokens dropped during Normalize.
var businessSuffixes = map[string]struct{}{
	"inc":  {},
	"ltd":  {},
	"llc":  {},
	"co":   {},
	"corp": {},
	"gmbh": {},
	"oy":   {},
	"ab":   {},
}

// Normalize lowercases s, strips diacritics, replaces non-alphanumeric runes
// with spaces, tokenizes on whitespace, and drops tokens in businessSuffixes.
// Returns a slice of tokens (may be empty).
func Normalize(s string) []string {
	return nil
}
