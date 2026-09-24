package activity

import "math"

// Display point budgets.
const (
	listTrackPoints   = 200
	detailRoutePoints = 4000
	streamPoints      = 600
)

// TrackCoordinates prepares the flattened [lat,lon,…] list served to the
// frontend. It returns mapAvailable false when the route must not be drawn at
// all: no GPS, the owner hid the route, or privacy trimming left too little.
//
// Every caller funnels through here, so the privacy pipeline is applied exactly
// once per response.
func TrackCoordinates(points []Point, routeHidden bool, zones []Zone, trimScope string, trimRadiusM, fullDistanceM float64, maxPoints int) ([]float64, bool) {
	if routeHidden {
		return nil, false
	}
	kept, hidden := TrimRoute(points, zones, trimScope, trimRadiusM, fullDistanceM)
	if hidden {
		return nil, false
	}
	kept = DecimateCoords(kept, maxPoints)
	return FlattenCoords(kept), true
}

// FlattenCoords renders points as [lat,lon,…] with five decimal places, which
// is about a metre of precision at any latitude.
func FlattenCoords(points []Point) []float64 {
	out := make([]float64, 0, len(points)*2)
	for _, point := range points {
		if point.Lat == nil || point.Lon == nil {
			continue
		}
		out = append(out, round5(*point.Lat), round5(*point.Lon))
	}
	return out
}

// Streams renders the charted series, decimated to at most streamPoints values
// each so the detail payload stays small.
func Streams(points []Point) (elapsed []int64, hr []*int64, cadence []*int64, power []*int64, speed []*float64, elevation []*float64) {
	strided := DecimateCoords(points, streamPoints)
	elapsed = make([]int64, 0, len(strided))
	hr = make([]*int64, 0, len(strided))
	cadence = make([]*int64, 0, len(strided))
	power = make([]*int64, 0, len(strided))
	speed = make([]*float64, 0, len(strided))
	elevation = make([]*float64, 0, len(strided))

	for _, point := range strided {
		elapsed = append(elapsed, point.ElapsedS)
		hr = append(hr, intPtr(point.HR))
		cadence = append(cadence, intPtr(point.Cad))
		power = append(power, intPtr(point.Pwr))
		speed = append(speed, floatPtr(point.Spd))
		elevation = append(elevation, floatPtr(point.Ele))
	}
	return elapsed, hr, cadence, power, speed, elevation
}

func round5(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return math.Round(value*100000) / 100000
}

func intPtr(value *int) *int64 {
	if value == nil {
		return nil
	}
	converted := int64(*value)
	return &converted
}

func floatPtr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	converted := *value
	return &converted
}
