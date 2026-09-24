package activity

import (
	"errors"
	"io"
	"math"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
)

// ErrNotAnActivity is returned for a valid FIT file that is not an activity.
var ErrNotAnActivity = errors.New("the FIT file does not contain an activity")

// ParseFIT decodes a FIT activity file.
func ParseFIT(r io.Reader) (ParsedActivity, error) {
	decoded, err := decoder.New(r).Decode()
	if err != nil {
		return ParsedActivity{}, err
	}
	if decoded.FileHeader.ProfileVersion == 0 {
		return ParsedActivity{}, errors.New("not a FIT file")
	}

	file := filedef.NewActivity(decoded.Messages...)
	if file.FileId.Type != typedef.FileActivity {
		return ParsedActivity{}, ErrNotAnActivity
	}

	parsed := ParsedActivity{}
	for _, record := range file.Records {
		if record.Timestamp.IsZero() {
			continue
		}
		parsed.Samples = append(parsed.Samples, recordSample(record))
	}
	if len(parsed.Samples) == 0 {
		return ParsedActivity{}, ErrNotAnActivity
	}
	parsed.StartedAt = parsed.Samples[0].T

	if len(file.Sessions) > 0 {
		session := file.Sessions[0]
		parsed.Sport = fitSport(session.Sport)
		if !session.StartTime.IsZero() {
			parsed.StartedAt = session.StartTime
		}
		if value := session.TotalElapsedTimeScaled(); !math.IsNaN(value) {
			seconds := int64(math.Round(value))
			parsed.SessionElapsedS = &seconds
		}
		if value := session.TotalTimerTimeScaled(); !math.IsNaN(value) {
			seconds := int64(math.Round(value))
			parsed.SessionTimerS = &seconds
		}
		if value := session.TotalDistanceScaled(); !math.IsNaN(value) {
			parsed.SessionDistanceM = &value
		}
		if session.TotalAscent != basetype.Uint16Invalid {
			ascent := float64(session.TotalAscent)
			parsed.SessionAscentM = &ascent
		}
		if session.TotalDescent != basetype.Uint16Invalid {
			descent := float64(session.TotalDescent)
			parsed.SessionDescentM = &descent
		}
		if session.AvgHeartRate != basetype.Uint8Invalid {
			hr := int(session.AvgHeartRate)
			parsed.SessionAvgHR = &hr
		}
		if session.MaxHeartRate != basetype.Uint8Invalid {
			hr := int(session.MaxHeartRate)
			parsed.SessionMaxHR = &hr
		}
	}
	if parsed.Sport == "" {
		parsed.Sport = SportOther
	}
	return parsed, nil
}

func recordSample(record *mesgdef.Record) Sample {
	sample := Sample{T: record.Timestamp.UTC()}

	if lat := record.PositionLatDegrees(); !math.IsNaN(lat) {
		sample.Lat = &lat
	}
	if lon := record.PositionLongDegrees(); !math.IsNaN(lon) {
		sample.Lon = &lon
	}
	// Enhanced altitude/speed are the modern fields; fall back when absent.
	if value := record.EnhancedAltitudeScaled(); !math.IsNaN(value) {
		sample.Ele = &value
	} else if value := record.AltitudeScaled(); !math.IsNaN(value) {
		sample.Ele = &value
	}
	if value := record.EnhancedSpeedScaled(); !math.IsNaN(value) {
		sample.Spd = &value
	} else if value := record.SpeedScaled(); !math.IsNaN(value) {
		sample.Spd = &value
	}
	if value := record.DistanceScaled(); !math.IsNaN(value) {
		sample.Dist = &value
	}
	if record.HeartRate != basetype.Uint8Invalid {
		hr := int(record.HeartRate)
		sample.HR = &hr
	}
	if record.Cadence != basetype.Uint8Invalid {
		cadence := int(record.Cadence)
		sample.Cad = &cadence
	}
	if record.Power != basetype.Uint16Invalid {
		power := int(record.Power)
		sample.Pwr = &power
	}
	return sample
}

// fitSport maps a FIT sport to a stored sport key. The FIT base-sport enum is
// coarse (trail and treadmill running are sub-sports of running), so every
// variant of a family maps to the same key.
func fitSport(sport typedef.Sport) string {
	switch sport {
	case typedef.SportRunning, typedef.SportWheelchairPushRun:
		return SportRun
	case typedef.SportCycling, typedef.SportEBiking:
		return SportRide
	case typedef.SportSwimming:
		return SportSwim
	case typedef.SportHiking, typedef.SportMountaineering:
		return SportHike
	case typedef.SportWalking, typedef.SportWheelchairPushWalk:
		return SportWalk
	case typedef.SportCrossCountrySkiing, typedef.SportAlpineSkiing, typedef.SportSnowboarding:
		return SportSki
	case typedef.SportRowing:
		return SportRow
	default:
		return SportOther
	}
}
