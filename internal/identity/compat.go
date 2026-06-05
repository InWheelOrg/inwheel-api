/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package identity

import (
	"sort"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// compat is the starter category-compatibility allowlist.
var compat = map[models.Category]map[models.Category]bool{
	models.CategoryCafe:       {models.CategoryCafe: true, models.CategoryRestaurant: true, models.CategoryBar: true},
	models.CategoryRestaurant: {models.CategoryRestaurant: true, models.CategoryCafe: true, models.CategoryBar: true},
	models.CategoryBar:        {models.CategoryBar: true, models.CategoryRestaurant: true, models.CategoryCafe: true},
	models.CategoryShop:       {models.CategoryShop: true},
	models.CategoryHealthcare: {models.CategoryHealthcare: true},
}

// Compatible returns the candidate categories that an incoming record of
// category c may match against.
func Compatible(c models.Category) []models.Category {
	set, ok := compat[c]
	if !ok {
		return []models.Category{c}
	}
	out := make([]models.Category, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
