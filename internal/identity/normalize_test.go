/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package identity_test

import (
	"reflect"
	"testing"

	"github.com/InWheelOrg/inwheel-api/internal/identity"
)

func TestNormalize(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"whitespace only", "   \t\n  ", nil},
		{"single word", "Pascal", []string{"pascal"}},
		{"lowercase + tokenize", "Café Pascal", []string{"cafe", "pascal"}},
		{"strip diacritics", "Açaí Bowl", []string{"acai", "bowl"}},
		{"punctuation becomes whitespace", "PASCAL CAFE.", []string{"pascal", "cafe"}},
		{"mixed separators", "Foo-Bar_Baz/Qux", []string{"foo", "bar", "baz", "qux"}},
		{"drops business suffix inc", "Pascal Inc", []string{"pascal"}},
		{"drops business suffix gmbh", "Beispiel GmbH", []string{"beispiel"}},
		{"drops multiple suffixes", "Acme Co Ltd", []string{"acme"}},
		{"preserves digits", "Cafe 22", []string{"cafe", "22"}},
		{"all-suffix input returns empty", "Inc Ltd Co", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := identity.Normalize(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Normalize(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
