package sync

import (
	"strings"

	"github.com/markbeep/hyl/internal/activity"
)

// stravaSportTypes is Strava's sport_type enumeration as published in the
// uploads documentation (checked 2026-09-24), keyed by its lowercase form. It is
// a whitelist rather than a translation table: Strava rejects an upload whose
// sport_type it does not recognise, so an unexpected provider value has to be
// dropped instead of passed through.
var stravaSportTypes = map[string]string{
	"alpineski":                     "AlpineSki",
	"backcountryski":                "BackcountrySki",
	"badminton":                     "Badminton",
	"basketball":                    "Basketball",
	"canoeing":                      "Canoeing",
	"cricket":                       "Cricket",
	"crossfit":                      "Crossfit",
	"dance":                         "Dance",
	"ebikeride":                     "EBikeRide",
	"elliptical":                    "Elliptical",
	"emountainbikeride":             "EMountainBikeRide",
	"golf":                          "Golf",
	"gravelride":                    "GravelRide",
	"handcycle":                     "Handcycle",
	"highintensityintervaltraining": "HighIntensityIntervalTraining",
	"hike":                          "Hike",
	"iceskate":                      "IceSkate",
	"inlineskate":                   "InlineSkate",
	"kayaking":                      "Kayaking",
	"kitesurf":                      "Kitesurf",
	"mountainbikeride":              "MountainBikeRide",
	"nordicski":                     "NordicSki",
	"padel":                         "Padel",
	"physicaltherapy":               "PhysicalTherapy",
	"pickleball":                    "Pickleball",
	"pilates":                       "Pilates",
	"racquetball":                   "Racquetball",
	"ride":                          "Ride",
	"rockclimbing":                  "RockClimbing",
	"rollerski":                     "RollerSki",
	"rowing":                        "Rowing",
	"run":                           "Run",
	"sail":                          "Sail",
	"skateboard":                    "Skateboard",
	"snowboard":                     "Snowboard",
	"snowshoe":                      "Snowshoe",
	"soccer":                        "Soccer",
	"squash":                        "Squash",
	"stairstepper":                  "StairStepper",
	"standuppaddling":               "StandUpPaddling",
	"surfing":                       "Surfing",
	"swim":                          "Swim",
	"tabletennis":                   "TableTennis",
	"tennis":                        "Tennis",
	"trailrun":                      "TrailRun",
	"velomobile":                    "Velomobile",
	"virtualride":                   "VirtualRide",
	"virtualrow":                    "VirtualRow",
	"virtualrun":                    "VirtualRun",
	"volleyball":                    "Volleyball",
	"walk":                          "Walk",
	"weighttraining":                "WeightTraining",
	"wheelchair":                    "Wheelchair",
	"windsurf":                      "Windsurf",
	"workout":                       "Workout",
	"yoga":                          "Yoga",
}

// StravaSportType picks the sport_type for an upload. The provider's own type
// wins whenever Strava knows it, so an imported gravel ride stays a gravel ride
// instead of collapsing into hyl's coarse "ride" key. Failing that, the five
// stored sports that map unambiguously are named explicitly. An empty result
// means "send no sport_type" and let Strava detect it from the file, which is
// the right answer for the ambiguous keys (ski, row, other).
func StravaSportType(sourceSport, storedSport string) string {
	if canonical, ok := stravaSportTypes[strings.ToLower(strings.TrimSpace(sourceSport))]; ok {
		return canonical
	}
	switch storedSport {
	case activity.SportRun:
		return "Run"
	case activity.SportRide:
		return "Ride"
	case activity.SportSwim:
		return "Swim"
	case activity.SportHike:
		return "Hike"
	case activity.SportWalk:
		return "Walk"
	default:
		return ""
	}
}
