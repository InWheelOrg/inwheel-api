/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package osm

import (
	"strconv"

	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// mapTagsToProfile derives an AccessibilityProfile from accessibility tags
// present on an OSM POI node. Returns nil when no accessibility signal is
// found. Reads only tags on the POI itself; no graph traversal.
func mapTagsToProfile(tags map[string]string) *models.AccessibilityProfile {
	overall, hasOverall := wheelchairToStatus(tags["wheelchair"])

	var components []models.A11yComponent
	if c, ok := mapRestroom(tags); ok {
		components = append(components, c)
	}
	if c, ok := mapParking(tags); ok {
		components = append(components, c)
	}
	if c, ok := mapEntrance(tags); ok {
		components = append(components, c)
	}
	if c, ok := mapElevator(tags); ok {
		components = append(components, c)
	}

	if !hasOverall && len(components) == 0 {
		return nil
	}
	if !hasOverall {
		overall = models.StatusUnknown
	}
	return &models.AccessibilityProfile{
		OverallStatus: overall,
		Components:    components,
	}
}

func wheelchairToStatus(v string) (models.A11yStatus, bool) {
	switch v {
	case "yes", "designated":
		return models.StatusAccessible, true
	case "limited":
		return models.StatusLimited, true
	case "no":
		return models.StatusInaccessible, true
	default:
		return "", false
	}
}

func mapRestroom(tags map[string]string) (models.A11yComponent, bool) {
	v, ok := tags["toilets:wheelchair"]
	if !ok {
		return models.A11yComponent{}, false
	}
	switch v {
	case "yes":
		yes := true
		return models.A11yComponent{
			Type:          models.ComponentRestroom,
			OverallStatus: models.StatusAccessible,
			Restroom:      &models.RestroomProperties{WheelchairAccessible: &yes},
		}, true
	case "no":
		no := false
		return models.A11yComponent{
			Type:          models.ComponentRestroom,
			OverallStatus: models.StatusInaccessible,
			Restroom:      &models.RestroomProperties{WheelchairAccessible: &no},
		}, true
	}
	return models.A11yComponent{}, false
}

func mapParking(tags map[string]string) (models.A11yComponent, bool) {
	if v, ok := tags["capacity:disabled"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return models.A11yComponent{}, false
		}
		if n > 0 {
			yes := true
			return models.A11yComponent{
				Type:          models.ComponentParking,
				OverallStatus: models.StatusAccessible,
				Parking: &models.ParkingProperties{
					HasDisabledSpaces: &yes,
					Count:             &n,
				},
			}, true
		}
		no := false
		return models.A11yComponent{
			Type:          models.ComponentParking,
			OverallStatus: models.StatusInaccessible,
			Parking:       &models.ParkingProperties{HasDisabledSpaces: &no},
		}, true
	}
	if tags["parking:disabled"] == "no" {
		no := false
		return models.A11yComponent{
			Type:          models.ComponentParking,
			OverallStatus: models.StatusInaccessible,
			Parking:       &models.ParkingProperties{HasDisabledSpaces: &no},
		}, true
	}
	return models.A11yComponent{}, false
}

func mapEntrance(tags map[string]string) (models.A11yComponent, bool) {
	props := &models.EntranceProperties{}
	any := false

	if v := tags["automatic_door"]; v != "" && v != "no" {
		t := true
		props.IsAutomatic = &t
		any = true
	}

	if stepCountPositive(tags["step_count"]) || stepCountPositive(tags["entrance:step_count"]) {
		t := true
		props.HasStep = &t
		any = true
	}

	// ramp:wheelchair takes precedence over the generic ramp key.
	if v, ok := tags["ramp:wheelchair"]; ok {
		switch v {
		case "yes":
			t := true
			props.HasRamp = &t
			any = true
		case "no":
			f := false
			props.HasRamp = &f
			any = true
		}
	} else if tags["ramp"] == "no" {
		f := false
		props.HasRamp = &f
		any = true
	}

	if !any {
		return models.A11yComponent{}, false
	}
	return models.A11yComponent{
		Type:          models.ComponentEntrance,
		OverallStatus: models.StatusUnknown,
		Entrance:      props,
	}, true
}

func stepCountPositive(v string) bool {
	if v == "" {
		return false
	}
	n, err := strconv.Atoi(v)
	return err == nil && n > 0
}

func mapElevator(tags map[string]string) (models.A11yComponent, bool) {
	if tags["elevator"] == "yes" {
		return models.A11yComponent{
			Type:          models.ComponentElevator,
			OverallStatus: models.StatusAccessible,
			Elevator:      &models.ElevatorProperties{},
		}, true
	}
	return models.A11yComponent{}, false
}
