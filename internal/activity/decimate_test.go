package activity

import (
	"testing"
	"time"
)

func coords(samples []Sample) int {
	count := 0
	for _, sample := range samples {
		if sample.HasCoords() {
			count++
		}
	}
	return count
}

func TestDecimateOneSamplePerSecond(t *testing.T) {
	var samples []Sample
	for i := range 100 {
		samples = append(samples, Sample{T: testStart.Add(time.Duration(i*100) * time.Millisecond)})
	}
	// 100 samples over 9.9 s at 10 Hz become one sample per one-second bucket,
	// each bucket keeping its last sample.
	kept := Decimate(samples)
	if len(kept) != 10 {
		t.Fatalf("decimated to %d samples, want 10", len(kept))
	}
	if kept[0].T != samples[9].T {
		t.Fatalf("first bucket kept %s, want the last sample of the bucket", kept[0].T)
	}
	for i := 1; i < len(kept); i++ {
		if gap := kept[i].T.Sub(kept[i-1].T); gap < time.Second {
			t.Fatalf("samples %d and %d are only %s apart", i-1, i, gap)
		}
	}
}

func TestDecimateCapsLongStreams(t *testing.T) {
	// 25 hours at 1 Hz is 90000 samples.
	total := 25 * 3600
	samples := make([]Sample, 0, total)
	for i := range total {
		samples = append(samples, Sample{T: testStart.Add(time.Duration(i) * time.Second)})
	}
	kept := Decimate(samples)
	if len(kept) > maxStoredPoints+1 {
		t.Fatalf("kept %d samples, want at most %d", len(kept), maxStoredPoints+1)
	}
	if kept[0].T != samples[0].T {
		t.Error("the first sample was dropped")
	}
	if kept[len(kept)-1].T != samples[len(samples)-1].T {
		t.Error("the last sample was dropped")
	}
}

func TestSimplifyStraightLine(t *testing.T) {
	var samples []Sample
	for i := range 200 {
		lat := 52.0 + float64(i)*0.00005
		lon := 5.0
		samples = append(samples, Sample{T: testStart.Add(time.Duration(i) * time.Second), Lat: &lat, Lon: &lon})
	}
	simplified := Simplify(samples)
	if got := coords(simplified); got != 2 {
		t.Fatalf("straight line kept %d coordinates, want 2", got)
	}
	if len(simplified) != len(samples) {
		t.Fatalf("simplification dropped rows: %d != %d", len(simplified), len(samples))
	}
	// Every other column survives, so streams stay intact.
	if simplified[10].T.IsZero() {
		t.Error("timestamps were lost")
	}
}

func TestSimplifyKeepsSignificantDetour(t *testing.T) {
	var samples []Sample
	for i := range 40 {
		lat := 52.0
		lon := 5.0 + float64(i)*0.00005
		if i == 20 {
			// about 40 m to the north of the straight line
			lat += 0.00036
		}
		samples = append(samples, Sample{T: testStart.Add(time.Duration(i) * time.Second), Lat: &lat, Lon: &lon})
	}
	simplified := Simplify(samples)
	if !simplified[20].HasCoords() {
		t.Fatal("the detour vertex was dropped")
	}
	if got := coords(simplified); got >= len(samples) {
		t.Fatalf("detour kept %d of %d coordinates, want a real reduction", got, len(samples))
	}
	// Both vertices of the step are needed to describe it, plus the endpoints.
	if got := coords(simplified); got > 6 {
		t.Fatalf("detour kept %d coordinates, want at most the step vertices", got)
	}
}

func TestDecimateCoordsKeepsEnds(t *testing.T) {
	coords := make([]float64, 1000)
	for i := range coords {
		coords[i] = float64(i)
	}
	out := DecimateCoords(coords, 200)
	if len(out) > 202 {
		t.Fatalf("kept %d values, want about 200", len(out))
	}
	if out[0] != 0 || out[len(out)-1] != 999 {
		t.Fatalf("ends lost: %v … %v", out[0], out[len(out)-1])
	}
}

func TestDecimateCoordsKeepsEachCoordinateOnce(t *testing.T) {
	// 4001 values at a budget of 4000 gives a stride of 2, which lands exactly
	// on the last index: appending the last value unconditionally would emit it
	// twice and exceed the budget, while the stride already covers it.
	coords := make([]float64, 4001)
	for i := range coords {
		coords[i] = float64(i)
	}
	out := DecimateCoords(coords, 4000)
	if len(out) > 4000 {
		t.Fatalf("kept %d values, want at most 4000", len(out))
	}
	if out[len(out)-1] != 4000 {
		t.Fatalf("last value = %v, want the final coordinate 4000", out[len(out)-1])
	}
	seen := make(map[float64]int, len(out))
	for _, value := range out {
		seen[value]++
	}
	for value, count := range seen {
		if count != 1 {
			t.Fatalf("coordinate %v emitted %d times", value, count)
		}
	}
}

func TestDecimateCoordsAppendsTheLastValueOffStride(t *testing.T) {
	// A stride that does not divide the last index still has to end on it.
	coords := make([]float64, 401)
	for i := range coords {
		coords[i] = float64(i)
	}
	out := DecimateCoords(coords, 400)
	if out[0] != 0 || out[len(out)-1] != 400 {
		t.Fatalf("ends lost: %v … %v", out[0], out[len(out)-1])
	}
}

func TestDedupeHashIsStableAcrossLocations(t *testing.T) {
	utc := time.Date(2026, 1, 2, 8, 0, 0, 0, time.UTC)
	amsterdam := utc.In(time.FixedZone("CET", 3600))
	if DedupeHash(utc, SportRide, 41250.4) != DedupeHash(amsterdam, SportRide, 41250.4) {
		t.Fatal("the same instant produced different hashes")
	}
	if DedupeHash(utc, SportRide, 41250.4) == DedupeHash(utc, SportRide, 41260.4) {
		t.Fatal("a different distance produced the same hash")
	}
	if DedupeHash(utc, SportRide, 1000) == DedupeHash(utc, SportRun, 1000) {
		t.Fatal("a different sport produced the same hash")
	}
}
