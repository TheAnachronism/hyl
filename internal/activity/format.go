package activity

import (
	"fmt"
	"strings"
	"time"
)

// The stored sport keys. They are exactly the keys the frontend's
// src/lib/sports.ts uses, so nothing translates between the two.
const (
	SportRun   = "run"
	SportRide  = "ride"
	SportSwim  = "swim"
	SportHike  = "hike"
	SportWalk  = "walk"
	SportSki   = "ski"
	SportRow   = "row"
	SportOther = "other"
)

var sportLabels = map[string]string{
	SportRun:   "Run",
	SportRide:  "Ride",
	SportSwim:  "Swim",
	SportHike:  "Hike",
	SportWalk:  "Walk",
	SportSki:   "Ski",
	SportRow:   "Row",
	SportOther: "Other",
}

// SportLabel renders a stored sport key for a generated title.
func SportLabel(key string) string {
	if label, ok := sportLabels[key]; ok {
		return label
	}
	return "Activity"
}

// SportKeys returns the stored sport keys in their canonical order. The import
// rules matrix and the frontend's sport picker both use it.
func SportKeys() []string {
	return []string{SportRun, SportRide, SportSwim, SportHike, SportWalk, SportSki, SportRow, SportOther}
}

// ValidSport reports whether a string is one of the stored sport keys.
func ValidSport(key string) bool {
	_, ok := sportLabels[key]
	return ok
}

// NormalizeSport maps any input to a stored sport key, falling back to other.
func NormalizeSport(raw string) string {
	key := strings.ToLower(strings.TrimSpace(raw))
	switch key {
	case SportRun, "running", "trail_running", "treadmill_running":
		return SportRun
	case SportRide, "cycling", "biking", "virtual_ride", "ebike_ride", "gravel_cycling", "mountain_biking":
		return SportRide
	case SportSwim, "swimming", "open_water_swimming":
		return SportSwim
	case SportHike, "hiking":
		return SportHike
	case SportWalk, "walking":
		return SportWalk
	case SportSki, "skiing", "nordic_ski", "alpine_ski", "backcountry_ski":
		return SportSki
	case SportRow, "rowing":
		return SportRow
	case SportOther, "":
		return SportOther
	default:
		return SportOther
	}
}

// DefaultTitle generates the title used when an upload carries none. Times are
// rendered in UTC so the same import produces the same title everywhere.
func DefaultTitle(sport string, startedAt time.Time) string {
	return fmt.Sprintf("%s on %s", SportLabel(sport), startedAt.UTC().Format("Mon 2 Jan 2006 15:04"))
}

// UsesPace reports whether a sport is displayed as min/km rather than km/h.
// This is the server-side twin of the frontend's sports.ts contract.
func UsesPace(sport string) bool {
	switch sport {
	case SportRun, SportWalk, SportHike:
		return true
	default:
		return false
	}
}
