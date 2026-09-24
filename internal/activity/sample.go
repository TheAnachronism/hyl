// Package activity parses FIT and GPX files, computes the metrics every list
// and detail view renders, applies the owner's route privacy rules and stores
// the resulting point stream.
package activity

import "time"

// Sample is one recorded point of an activity, shared by the FIT and GPX
// readers. Every optional field is a pointer so "not recorded" stays
// distinguishable from zero.
type Sample struct {
	T    time.Time
	Lat  *float64 // degrees
	Lon  *float64
	Ele  *float64 // metres
	HR   *int
	Cad  *int
	Pwr  *int
	Spd  *float64 // m/s
	Dist *float64 // cumulative metres
}

// ParsedActivity is what a reader produces before metrics are computed.
type ParsedActivity struct {
	StartedAt time.Time
	Sport     string
	Samples   []Sample

	// Device-reported session totals; nil when the file does not carry them.
	SessionElapsedS  *int64
	SessionTimerS    *int64
	SessionDistanceM *float64
	SessionAscentM   *float64
	SessionDescentM  *float64
	SessionAvgHR     *int
	SessionMaxHR     *int
}

// HasCoords reports whether a sample carries a position.
func (s Sample) HasCoords() bool { return s.Lat != nil && s.Lon != nil }

// Point is one stored activity_points row.
type Point struct {
	Seq      int64
	T        int64 // unix seconds UTC
	ElapsedS int64 // seconds since StartedAt, pauses included
	Lat      *float64
	Lon      *float64
	Ele      *float64
	HR       *int
	Cad      *int
	Pwr      *int
	Spd      *float64
	DistM    *float64
}

// HasCoords reports whether a point carries a drawable position.
func (p Point) HasCoords() bool { return p.Lat != nil && p.Lon != nil }

// SamplePoints converts parsed samples to storable points. Samples without a
// timestamp are the caller's responsibility to drop.
func SamplePoints(startedAt time.Time, samples []Sample) []Point {
	points := make([]Point, 0, len(samples))
	for i, sample := range samples {
		points = append(points, Point{
			Seq:      int64(i),
			T:        sample.T.Unix(),
			ElapsedS: int64(sample.T.Sub(startedAt).Seconds()),
			Lat:      sample.Lat,
			Lon:      sample.Lon,
			Ele:      sample.Ele,
			HR:       sample.HR,
			Cad:      sample.Cad,
			Pwr:      sample.Pwr,
			Spd:      sample.Spd,
			DistM:    sample.Dist,
		})
	}
	return points
}
