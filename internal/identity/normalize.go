/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package identity

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

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
// Returns nil for empty or all-suffix input.
func Normalize(s string) []string {
	t := transform.Chain(norm.NFKD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	folded, _, err := transform.String(t, s)
	if err != nil {
		folded = s
	}
	folded = strings.ToLower(folded)

	var b strings.Builder
	b.Grow(len(folded))
	for _, r := range folded {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}

	fields := strings.Fields(b.String())
	if len(fields) == 0 {
		return nil
	}
	out := make([]string, 0, len(fields))
	for _, tok := range fields {
		if _, drop := businessSuffixes[tok]; drop {
			continue
		}
		out = append(out, tok)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
