package activity

import (
	"math"
	"time"
)

// Storage limits. Decimation runs before storage, so activity_points never
// exceeds maxStoredPoints rows per activity.
const (
	maxStoredPoints = 20000
	rdpEpsilonM     = 5.0
)

// Decimate keeps at most one sample per second (keeping the later sample of a
// cluster) and then caps the series at maxStoredPoints by uniform striding,
// always preserving the first and last sample.
func Decimate(samples []Sample) []Sample {
	if len(samples) == 0 {
		return nil
	}
	// Buckets are anchored on the first sample and each bucket keeps its last
	// sample, so the result is a 1 Hz series rather than a sliding window.
	kept := make([]Sample, 0, len(samples))
	bucket := samples[0]
	bucketStart := samples[0].T
	for _, sample := range samples[1:] {
		if sample.T.Sub(bucketStart) < time.Second {
			bucket = sample
			continue
		}
		kept = append(kept, bucket)
		bucket, bucketStart = sample, sample.T
	}
	return capSamples(append(kept, bucket))
}

// capSamples enforces the per-activity storage ceiling.
func capSamples(kept []Sample) []Sample {
	if len(kept) <= maxStoredPoints {
		return kept
	}

	step := (len(kept) + maxStoredPoints - 1) / maxStoredPoints
	capped := make([]Sample, 0, maxStoredPoints+1)
	for i := 0; i < len(kept); i += step {
		capped = append(capped, kept[i])
	}
	if last := kept[len(kept)-1]; capped[len(capped)-1].T != last.T {
		capped = append(capped, last)
	}
	return capped
}

// Simplify runs Ramer–Douglas–Peucker with a 5 m tolerance over the
// coordinate-carrying samples and stores nil coordinates for the points it
// drops. Every other column survives, so the streams stay intact while the
// route compresses; distance is unaffected because it comes from dist_m.
func Simplify(samples []Sample) []Sample {
	indexes := make([]int, 0, len(samples))
	for i := range samples {
		if samples[i].HasCoords() {
			indexes = append(indexes, i)
		}
	}
	if len(indexes) < 3 {
		return samples
	}

	lat0 := *samples[indexes[0]].Lat
	lon0 := *samples[indexes[0]].Lon
	mPerDegLon := 111320 * math.Cos(lat0*math.Pi/180)
	const mPerDegLat = 110540

	projected := make([][2]float64, len(indexes))
	for k, i := range indexes {
		projected[k] = [2]float64{
			(*samples[i].Lon - lon0) * mPerDegLon,
			(*samples[i].Lat - lat0) * mPerDegLat,
		}
	}

	keep := make([]bool, len(projected))
	keep[0] = true
	keep[len(keep)-1] = true
	rdp(projected, 0, len(projected)-1, rdpEpsilonM, keep)

	out := make([]Sample, len(samples))
	copy(out, samples)
	for k, i := range indexes {
		if !keep[k] {
			out[i].Lat = nil
			out[i].Lon = nil
		}
	}
	return out
}

// rdp marks the vertices that must survive, using an explicit stack so a long
// route cannot exhaust the goroutine stack.
func rdp(points [][2]float64, first, last int, epsilon float64, keep []bool) {
	type span struct{ a, b int }
	stack := []span{{first, last}}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current.b <= current.a+1 {
			continue
		}
		worst, worstIndex := 0.0, -1
		for i := current.a + 1; i < current.b; i++ {
			distance := perpendicularDistance(points[i], points[current.a], points[current.b])
			if distance > worst {
				worst, worstIndex = distance, i
			}
		}
		if worstIndex >= 0 && worst > epsilon {
			keep[worstIndex] = true
			stack = append(stack, span{current.a, worstIndex}, span{worstIndex, current.b})
		}
	}
}

// perpendicularDistance is the distance from p to the segment a–b in the local
// projection the caller already applied.
func perpendicularDistance(p, a, b [2]float64) float64 {
	dx := b[0] - a[0]
	dy := b[1] - a[1]
	if dx == 0 && dy == 0 {
		return math.Hypot(p[0]-a[0], p[1]-a[1])
	}
	t := ((p[0]-a[0])*dx + (p[1]-a[1])*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(p[0]-(a[0]+t*dx), p[1]-(a[1]+t*dy))
}

// DecimateCoords reduces a coordinate list for display, always keeping the
// first and last coordinate.
func DecimateCoords[T any](coords []T, maxPoints int) []T {
	if maxPoints <= 2 || len(coords) <= maxPoints {
		return coords
	}
	step := (len(coords) + maxPoints - 1) / maxPoints
	out := make([]T, 0, maxPoints+1)
	for i := 0; i < len(coords); i += step {
		out = append(out, coords[i])
	}
	// The stride can land exactly on the last index. Appending it then would
	// duplicate that coordinate and put the result one over the budget, which
	// is a zero-length polyline segment and a repeated stream value.
	if last := len(coords) - 1; last%step != 0 {
		out = append(out, coords[last])
	}
	return out
}
