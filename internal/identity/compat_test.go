/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package identity_test

import (
	"reflect"
	"testing"

	"github.com/InWheelOrg/inwheel-api/internal/identity"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

func TestCompatible(t *testing.T) {
	cases := []struct {
		name string
		in   models.Category
		want []models.Category
	}{
		{
			name: "cafe maps to food/drink cluster, sorted",
			in:   models.CategoryCafe,
			want: []models.Category{models.CategoryBar, models.CategoryCafe, models.CategoryRestaurant},
		},
		{
			name: "restaurant maps to food/drink cluster, sorted",
			in:   models.CategoryRestaurant,
			want: []models.Category{models.CategoryBar, models.CategoryCafe, models.CategoryRestaurant},
		},
		{
			name: "shop maps to self only",
			in:   models.CategoryShop,
			want: []models.Category{models.CategoryShop},
		},
		{
			name: "healthcare maps to self only",
			in:   models.CategoryHealthcare,
			want: []models.Category{models.CategoryHealthcare},
		},
		{
			name: "category not in map returns self only",
			in:   models.CategoryMall,
			want: []models.Category{models.CategoryMall},
		},
		{
			name: "empty category returns self only",
			in:   "",
			want: []models.Category{""},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := identity.Compatible(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Compatible(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
