/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package osm

import (
	"strconv"
	"strings"
	"time"

	"github.com/InWheelOrg/inwheel-api/internal/a11y"
	"github.com/InWheelOrg/inwheel-api/pkg/models"
)

// mapTagsToProfile derives an AccessibilityProfile from OSM accessibility tags
// on a POI node. Returns nil when no accessibility signal is found.
// The OSM wheelchair tag is stored as a SourceReport, not interpreted as truth.
func mapTagsToProfile(tags map[string]string) *models.AccessibilityProfile {
	var reports models.SourceReports
	if v, ok := tags["wheelchair"]; ok {
		switch v {
		case "yes", "designated", "limited", "no":
			reports = append(reports, models.SourceReport{
				Source:     "osm",
				Value:      v,
				RecordedAt: time.Now(),
			})
		}
	}

	entrance := mapEntrance(tags)
	restroom := mapRestroom(tags)
	parking := mapParking(tags)
	elevator := mapElevator(tags)

	if len(reports) == 0 && entrance == nil && restroom == nil && parking == nil && elevator == nil {
		return nil
	}

	return &models.AccessibilityProfile{
		SourceReports: reports,
		Entrance:      entrance,
		Restroom:      restroom,
		Parking:       parking,
		Elevator:      elevator,
	}
}

func mapRestroom(tags map[string]string) *models.RestroomProps {
	v, ok := tags["toilets:wheelchair"]
	if !ok {
		return nil
	}
	switch v {
	case "yes":
		t := true
		return &models.RestroomProps{IsAccessible: &t}
	case "no":
		f := false
		return &models.RestroomProps{IsAccessible: &f}
	}
	return nil
}

func mapParking(tags map[string]string) *models.ParkingProps {
	if v, ok := tags["capacity:disabled"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil
		}
		hasSpaces := n > 0
		return &models.ParkingProps{HasDisabledSpaces: &hasSpaces, Count: &n}
	}
	if tags["parking:disabled"] == "no" {
		f := false
		return &models.ParkingProps{HasDisabledSpaces: &f}
	}
	return nil
}

func mapEntrance(tags map[string]string) *models.EntranceProps {
	props := &models.EntranceProps{}
	found := false

	if v := tags["automatic_door"]; v != "" && v != "no" {
		auto := models.DoorAutomatic
		if props.Door == nil {
			props.Door = &models.DoorProps{}
		}
		props.Door.Type = auto
		found = true
	}

	if stepCountPositive(tags["step_count"]) || stepCountPositive(tags["entrance:step_count"]) {
		f := false
		props.IsLevel = &f
		found = true
	}

	if v, ok := tags["ramp:wheelchair"]; ok {
		switch v {
		case "yes":
			t := true
			props.HasFixedRamp = &t
			found = true
		case "no":
			f := false
			props.HasFixedRamp = &f
			found = true
		}
	} else if tags["ramp"] == "no" {
		f := false
		props.HasFixedRamp = &f
		found = true
	}

	if v, ok := parseMetres(tags["width"], tags["door:width"]); ok {
		level := a11y.LevelForDoorWidth(v)
		props.Width = &level
		found = true
	}

	if v, ok := parsePercent(tags["incline"]); ok {
		level := a11y.LevelForSlopePercent(v)
		props.SlopePercent = &level
		found = true
	}

	if !found {
		return nil
	}
	return props
}

func stepCountPositive(v string) bool {
	if v == "" {
		return false
	}
	n, err := strconv.Atoi(v)
	return err == nil && n > 0
}

func parseMetres(tags ...string) (float64, bool) {
	for _, v := range tags {
		if v == "" {
			continue
		}
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

func parsePercent(v string) (float64, bool) {
	v = strings.TrimSpace(v)
	if !strings.HasSuffix(v, "%") {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
	if err != nil {
		return 0, false
	}
	if f < 0 {
		f = -f
	}
	return f, true
}

func mapElevator(tags map[string]string) *models.ElevatorProps {
	if tags["elevator"] == "yes" {
		return &models.ElevatorProps{}
	}
	return nil
}
