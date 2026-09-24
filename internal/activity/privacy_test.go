package activity

import (
	"math"
	"testing"
	"time"
)

// straightRoute builds a route of n points spaced stepM apart on a heading of
// roughly north, which makes the distances easy to reason about.
func straightRoute(n int, stepM float64) []Point {
	const mPerDegLat = 110540.0
	points := make([]Point, 0, n)
	for i := range n {
		lat := 52.0 + float64(i)*stepM/mPerDegLat
		lon := 5.0
		distance := float64(i) * stepM
		points = append(points, Point{
			Seq:      int64(i),
			T:        testStart.Add(time.Duration(i) * time.Second).Unix(),
			ElapsedS: int64(i),
			Lat:      &lat,
			Lon:      &lon,
			DistM:    &distance,
		})
	}
	return points
}

func routeLengthM(points []Point) float64 {
	total := 0.0
	for i := 1; i < len(points); i++ {
		if points[i-1].Lat == nil || points[i].Lat == nil {
			continue
		}
		total += HaversineM(*points[i-1].Lat, *points[i-1].Lon, *points[i].Lat, *points[i].Lon)
	}
	return total
}

func TestTrimRouteByDistance(t *testing.T) {
	// 10 km sampled every 100 m, 200 m trimmed from both ends.
	points := straightRoute(101, 100)
	kept, hidden := TrimRoute(points, nil, "all", 200, 10000)
	if hidden {
		t.Fatal("a 10 km route was hidden after a 200 m trim")
	}

	first, last := points[0], points[len(points)-1]
	leading := HaversineM(*kept[0].Lat, *kept[0].Lon, *first.Lat, *first.Lon)
	trailing := HaversineM(*kept[len(kept)-1].Lat, *kept[len(kept)-1].Lon, *last.Lat, *last.Lon)
	// The boundary is interpolated onto the polyline, so the first and last
	// served coordinates sit exactly at the radius, never inside it.
	if math.Abs(leading-200) > 1 || math.Abs(trailing-200) > 1 {
		t.Fatalf("boundaries are %.0f m / %.0f m from the ends, want 200 m", leading, trailing)
	}
	total := routeLengthM(points)
	if got := routeLengthM(kept); math.Abs(got-(total-400)) > 2 {
		t.Fatalf("kept %.0f m, want %.0f m", got, total-400)
	}
	// Every stored point that was outside the radius survives.
	if len(kept) < len(points)-4 {
		t.Fatalf("kept %d points, want at least %d", len(kept), len(points)-4)
	}
	// Trimming never touches the metrics, only the coordinates.
	if points[0].DistM == nil || *points[0].DistM != 0 {
		t.Fatal("trimming mutated the source points")
	}
	if len(points) != 101 {
		t.Fatalf("the full stream has %d points, want 101", len(points))
	}
}

func TestTrimRouteHidesShortRemainder(t *testing.T) {
	t.Run("below the 50 m floor", func(t *testing.T) {
		points := straightRoute(3, 50) // 100 m long
		if _, hidden := TrimRoute(points, nil, "all", 60, 100); !hidden {
			t.Fatal("a 100 m route with a 60 m radius was not hidden")
		}
	})

	t.Run("below 30 percent", func(t *testing.T) {
		points := straightRoute(101, 100)
		total := routeLengthM(points)
		// A 3.6 km trim at each end leaves about 2.8 km of a 10 km route.
		if _, hidden := TrimRoute(points, nil, "all", 3600, total); !hidden {
			t.Fatal("a 3.6 km trim of a 10 km route was not hidden")
		}
	})

	t.Run("fewer than two coordinates", func(t *testing.T) {
		if _, hidden := TrimRoute([]Point{{Seq: 0}}, nil, "all", 100, 1000); !hidden {
			t.Fatal("a single-point route was not hidden")
		}
	})
}

func TestTrimRouteByZones(t *testing.T) {
	points := straightRoute(101, 100)
	// A zone over the first kilometre of the route.
	zones := []Zone{{Lat: *points[0].Lat, Lon: *points[0].Lon, RadiusM: 1000}}

	kept, hidden := TrimRoute(points, zones, "zones", 0, routeLengthM(points))
	if hidden {
		t.Fatal("a zone trim of the first kilometre hid the whole route")
	}

	first, last := points[0], points[len(points)-1]
	leading := HaversineM(*kept[0].Lat, *kept[0].Lon, *first.Lat, *first.Lon)
	if leading < 1000 {
		t.Fatalf("kept a point %.0f m from the start, want at least 1000 m", leading)
	}
	_ = leading
	// The tail is untouched: the last point survives.
	if trailing := HaversineM(*kept[len(kept)-1].Lat, *kept[len(kept)-1].Lon, *last.Lat, *last.Lon); trailing != 0 {
		t.Fatalf("the tail was trimmed by %.0f m", trailing)
	}
	if len(kept) != 91 {
		t.Fatalf("kept %d points, want 91", len(kept))
	}
}

func TestTrackCoordinatesHonoursRouteHidden(t *testing.T) {
	points := straightRoute(101, 100)
	coords, available := TrackCoordinates(points, true, nil, "all", 200, 10000, listTrackPoints)
	if available || coords != nil {
		t.Fatalf("routeHidden produced %v/%v", coords, available)
	}

	coords, available = TrackCoordinates(points, false, nil, "all", 0, routeLengthM(points), listTrackPoints)
	if !available {
		t.Fatal("a visible route was reported as unavailable")
	}
	if len(coords) != 202 {
		t.Fatalf("flattened %d values, want 202", len(coords))
	}
	if math.Abs(coords[0]-52.0) > 1e-5 || math.Abs(coords[1]-5.0) > 1e-5 {
		t.Fatalf("the first coordinate is wrong: %v", coords[:2])
	}
}

func TestTrimRouteAnchorsOnFirstAndLast(t *testing.T) {
	// A route that loops back to its start must not be trimmed by the radius
	// alone: the anchor is the endpoint, not the nearest point.
	points := straightRoute(21, 100)
	points[10].Lat = points[0].Lat
	points[10].Lon = points[0].Lon
	kept, hidden := TrimRoute(points, nil, "all", 250, 2000)
	if hidden {
		t.Fatal("loop-back route was hidden")
	}
	if len(kept) == 0 {
		t.Fatal("nothing was kept")
	}
}
