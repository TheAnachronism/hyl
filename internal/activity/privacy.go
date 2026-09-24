package activity

import "math"

// Hide thresholds: a trimmed route is not drawn at all when too little of it
// would remain.
const (
	minVisibleRouteM   = 50.0
	minVisibleRoutePct = 0.30
)

// Zone is one circular privacy zone in the owner's settings.
type Zone struct {
	Lat     float64
	Lon     float64
	RadiusM float64
}

// TrimRoute applies the owner's start/end privacy trimming to a coordinate list
// and reports whether the remaining route is too short to display.
//
// scope is "all" (cut radiusM off both ends, measured along the polyline) or
// "zones" (cut the prefix and suffix that lie inside one of the owner's privacy
// zones). Only the coordinates are affected: metrics and streams are returned
// untouched by the handlers.
//
// With scope "all" the two new ends are interpolated onto the polyline instead
// of snapping to the nearest stored point. That matters because stored routes
// are simplified: a straight 12 km road can survive with nothing but its two
// endpoints, and snapping would have to drop the whole route rather than trim
// 200 m from each end.
//
// fullDistanceM is the activity's stored distance and drives the "less than
// 30 % of the route survived" rule.
func TrimRoute(pts []Point, zones []Zone, scope string, radiusM, fullDistanceM float64) (kept []Point, hidden bool) {
	coords := make([]Point, 0, len(pts))
	for i := range pts {
		if pts[i].HasCoords() {
			coords = append(coords, pts[i])
		}
	}
	if len(coords) < 2 {
		return nil, true
	}

	cumulative := make([]float64, len(coords))
	for i := 1; i < len(coords); i++ {
		cumulative[i] = cumulative[i-1] + HaversineM(
			*coords[i-1].Lat, *coords[i-1].Lon, *coords[i].Lat, *coords[i].Lon)
	}
	total := cumulative[len(cumulative)-1]

	if scope == "zones" {
		return trimByZones(coords, cumulative, zones, fullDistanceM)
	}

	// radiusM is capped at half the route, so a route shorter than the radius
	// is hidden rather than half-drawn.
	cut := math.Min(radiusM, total/2)
	startDistance, endDistance := cut, total-cut

	keptDistance := endDistance - startDistance
	if keptDistance < minVisibleRouteM {
		return nil, true
	}
	if fullDistanceM > 0 && keptDistance < minVisibleRoutePct*fullDistanceM {
		return nil, true
	}
	return pointsBetween(coords, cumulative, startDistance, endDistance), false
}

// trimByZones drops the contiguous prefix and suffix that sit inside a privacy
// zone. A zone in the middle of a route stops the scan at that point, so it
// never cuts a hole; a route that starts and ends inside zones is hidden.
func trimByZones(coords []Point, cumulative []float64, zones []Zone, fullDistanceM float64) ([]Point, bool) {
	if len(zones) == 0 {
		return coords, false
	}
	insideZone := func(point Point) bool {
		for _, zone := range zones {
			if HaversineM(*point.Lat, *point.Lon, zone.Lat, zone.Lon) <= zone.RadiusM {
				return true
			}
		}
		return false
	}

	start := 0
	for start < len(coords)-1 && insideZone(coords[start]) {
		start++
	}
	end := len(coords) - 1
	for end > start && insideZone(coords[end]) {
		end--
	}
	if end <= start {
		return nil, true
	}

	keptDistance := cumulative[end] - cumulative[start]
	if keptDistance < minVisibleRouteM {
		return nil, true
	}
	if fullDistanceM > 0 && keptDistance < minVisibleRoutePct*fullDistanceM {
		return nil, true
	}

	out := make([]Point, 0, end-start+1)
	out = append(out, coords[start:end+1]...)
	return out, false
}

// pointsBetween returns the polyline between two distances along it: the stored
// points strictly inside, with both boundaries interpolated unless they fall
// exactly on the stored ends.
func pointsBetween(coords []Point, cumulative []float64, startDistance, endDistance float64) []Point {
	total := cumulative[len(cumulative)-1]
	out := make([]Point, 0, len(coords)+2)

	if startDistance <= 0 {
		out = append(out, coords[0])
	} else {
		out = append(out, interpolateAt(coords, cumulative, startDistance))
	}
	for i := range coords {
		if cumulative[i] <= startDistance || cumulative[i] >= endDistance {
			continue
		}
		out = append(out, coords[i])
	}
	if endDistance >= total {
		out = append(out, coords[len(coords)-1])
	} else {
		out = append(out, interpolateAt(coords, cumulative, endDistance))
	}
	return out
}

// interpolateAt places a point exactly at one distance along the polyline.
func interpolateAt(coords []Point, cumulative []float64, distance float64) Point {
	index := 1
	for index < len(cumulative)-1 && cumulative[index] < distance {
		index++
	}
	previous := cumulative[index-1]
	span := cumulative[index] - previous
	ratio := 0.0
	if span > 0 {
		ratio = (distance - previous) / span
	}
	lat := *coords[index-1].Lat + ratio*(*coords[index].Lat-*coords[index-1].Lat)
	lon := *coords[index-1].Lon + ratio*(*coords[index].Lon-*coords[index-1].Lon)

	point := coords[index-1]
	point.Seq = -1
	point.Lat, point.Lon = &lat, &lon
	point.ElapsedS = coords[index-1].ElapsedS +
		int64(ratio*float64(coords[index].ElapsedS-coords[index-1].ElapsedS))
	point.T = coords[index-1].T +
		int64(ratio*float64(coords[index].T-coords[index-1].T))
	return point
}
