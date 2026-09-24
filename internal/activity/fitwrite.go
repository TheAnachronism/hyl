package activity

import (
	"io"
	"math"
	"time"

	"github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/markbeep/hyl/internal/db"
)

// WriteFIT synthesizes a minimal activity FIT from stored points. hyl never
// keeps the original upload, so an export has to be re-encoded from our own
// stream; coordinates dropped by simplification are linearly interpolated
// between the nearest kept anchors, and points outside the first/last anchor
// are omitted entirely.
func WriteFIT(w io.Writer, activity db.Activity, points []Point) error {
	start := time.Unix(activity.StartedAt, 0).UTC()
	end := start.Add(time.Duration(activity.ElapsedTimeS) * time.Second)
	sport := fitSportValue(activity.Sport)

	file := filedef.NewActivity()
	file.FileId.
		SetType(typedef.FileActivity).
		SetManufacturer(typedef.ManufacturerGarmin).
		SetProduct(1).
		SetSerialNumber(1).
		SetTimeCreated(start)

	session := mesgdef.NewSession(nil).
		SetTimestamp(end).
		SetStartTime(start).
		SetSport(sport).
		SetTotalElapsedTimeScaled(float64(activity.ElapsedTimeS)).
		SetTotalTimerTimeScaled(float64(activity.MovingTimeS)).
		SetTotalDistanceScaled(activity.DistanceM).
		SetTotalAscent(clampUint16(activity.ElevationGainM)).
		SetTotalDescent(clampUint16(activity.ElevationLossM))
	if activity.AvgSpeedMps != nil {
		session.SetAvgSpeedScaled(*activity.AvgSpeedMps)
	}
	if activity.MaxSpeedMps != nil {
		session.SetMaxSpeedScaled(*activity.MaxSpeedMps)
	}
	if activity.AvgHeartRate != nil {
		session.SetAvgHeartRate(clampUint8(float64(*activity.AvgHeartRate)))
	}
	if activity.MaxHeartRate != nil {
		session.SetMaxHeartRate(clampUint8(float64(*activity.MaxHeartRate)))
	}
	if activity.AvgCadence != nil {
		session.SetAvgCadence(clampUint8(*activity.AvgCadence))
	}
	file.Sessions = append(file.Sessions, session)

	lap := mesgdef.NewLap(nil).
		SetTimestamp(end).
		SetStartTime(start).
		SetSport(sport).
		SetTotalElapsedTimeScaled(float64(activity.ElapsedTimeS)).
		SetTotalTimerTimeScaled(float64(activity.MovingTimeS)).
		SetTotalDistanceScaled(activity.DistanceM)
	file.Laps = append(file.Laps, lap)

	file.Activity = mesgdef.NewActivity(nil).
		SetType(typedef.ActivityManual).
		SetTimestamp(end).
		SetNumSessions(1)

	for _, point := range recordsWithInterpolatedPositions(points) {
		// A field left at its invalid sentinel is omitted by the encoder, so
		// only present values are set.
		record := mesgdef.NewRecord(nil).SetTimestamp(time.Unix(point.T, 0).UTC())
		if point.Lat != nil && point.Lon != nil {
			record.SetPositionLatDegrees(*point.Lat).SetPositionLongDegrees(*point.Lon)
		}
		if point.DistM != nil {
			record.SetDistanceScaled(*point.DistM)
		}
		if point.Spd != nil {
			record.SetEnhancedSpeedScaled(*point.Spd)
		}
		if point.Ele != nil {
			record.SetEnhancedAltitudeScaled(*point.Ele)
		}
		if point.HR != nil {
			record.SetHeartRate(clampUint8(float64(*point.HR)))
		}
		if point.Cad != nil {
			record.SetCadence(clampUint8(float64(*point.Cad)))
		}
		if point.Pwr != nil {
			record.SetPower(clampUint16(float64(*point.Pwr)))
		}
		file.Records = append(file.Records, record)
	}

	fit := file.ToFIT(nil)
	return encoder.New(w).Encode(&fit)
}

// recordsWithInterpolatedPositions fills in the coordinates of points that
// simplification stripped, keeping only the span covered by real anchors.
func recordsWithInterpolatedPositions(points []Point) []Point {
	anchors := make([]int, 0, len(points))
	for i := range points {
		if points[i].HasCoords() {
			anchors = append(anchors, i)
		}
	}
	if len(anchors) == 0 {
		return nil
	}

	out := make([]Point, 0, len(points))
	next := 0
	for i := anchors[0]; i <= anchors[len(anchors)-1]; i++ {
		point := points[i]
		if point.HasCoords() {
			out = append(out, point)
			for next < len(anchors) && anchors[next] <= i {
				next++
			}
			continue
		}
		prev := anchors[max(0, next-1)]
		following := anchors[min(len(anchors)-1, next)]
		prevPoint, nextPoint := points[prev], points[following]
		if prevPoint.Lat == nil || prevPoint.Lon == nil || nextPoint.Lat == nil || nextPoint.Lon == nil ||
			nextPoint.ElapsedS == prevPoint.ElapsedS {
			continue
		}
		ratio := float64(point.ElapsedS-prevPoint.ElapsedS) / float64(nextPoint.ElapsedS-prevPoint.ElapsedS)
		lat := *prevPoint.Lat + ratio*(*nextPoint.Lat-*prevPoint.Lat)
		lon := *prevPoint.Lon + ratio*(*nextPoint.Lon-*prevPoint.Lon)
		point.Lat, point.Lon = &lat, &lon
		out = append(out, point)
	}
	return out
}

func clampUint8(value float64) uint8 {
	if value <= 0 || math.IsNaN(value) {
		return 0
	}
	if value >= 255 {
		return 254
	}
	return uint8(math.Round(value))
}

func clampUint16(value float64) uint16 {
	if value <= 0 || math.IsNaN(value) {
		return 0
	}
	if value >= 65535 {
		return 65534
	}
	return uint16(math.Round(value))
}

// fitSportValue maps a stored sport key back to a FIT sport.
func fitSportValue(key string) typedef.Sport {
	switch key {
	case SportRun:
		return typedef.SportRunning
	case SportRide:
		return typedef.SportCycling
	case SportSwim:
		return typedef.SportSwimming
	case SportHike:
		return typedef.SportHiking
	case SportWalk:
		return typedef.SportWalking
	case SportSki:
		return typedef.SportCrossCountrySkiing
	case SportRow:
		return typedef.SportRowing
	default:
		return typedef.SportGeneric
	}
}
