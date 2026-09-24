package activity

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/markbeep/hyl/internal/db"
)

// TestParseRealFITFiles decodes every real device file in the repository's
// example/ directory. Real files are the only way to catch the field quirks a
// synthetic fixture cannot produce (enhanced vs legacy speed and altitude,
// invalid sentinels, session totals); the test skips when the directory is
// absent, so a checkout without them still builds.
func TestParseRealFITFiles(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "..", "example", "*.fit"))
	if err != nil {
		t.Fatalf("globbing example files: %v", err)
	}
	if len(matches) == 0 {
		t.Skip("no FIT files in example/")
	}

	var sportCounts = map[string]int{}
	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			file, err := os.Open(path)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer func() { _ = file.Close() }()

			parsed, err := ParseFIT(file)
			if err != nil {
				t.Fatalf("ParseFIT: %v", err)
			}
			if len(parsed.Samples) == 0 {
				t.Fatal("no samples decoded")
			}
			if !ValidSport(parsed.Sport) {
				t.Fatalf("sport %q is not a stored key", parsed.Sport)
			}
			sportCounts[parsed.Sport]++

			metrics := ComputeMetrics(parsed)
			if metrics.ElapsedTimeS <= 0 {
				t.Errorf("elapsed time = %d", metrics.ElapsedTimeS)
			}
			if metrics.MovingTimeS > metrics.ElapsedTimeS {
				t.Errorf("moving %d > elapsed %d", metrics.MovingTimeS, metrics.ElapsedTimeS)
			}
			if metrics.DistanceM <= 0 {
				t.Errorf("distance = %.1f", metrics.DistanceM)
			}
			avgSpeed := metrics.DistanceM / float64(metrics.ElapsedTimeS)
			if metrics.ElapsedTimeS > 0 && avgSpeed > 25 {
				t.Errorf("implausible average speed %.1f m/s", avgSpeed)
			}
			if metrics.HasGPS && metrics.DistanceM == 0 {
				t.Error("has_gps with no distance")
			}

			// Decimation and simplification must produce a storable stream that
			// still covers the whole activity.
			stored := Simplify(Decimate(parsed.Samples))
			if len(stored) > maxStoredPoints+1 {
				t.Errorf("stored %d samples, over the cap", len(stored))
			}
			if stored[0].T != parsed.Samples[0].T && !stored[0].T.Equal(parsed.Samples[0].T) {
				t.Error("decimation dropped the first sample")
			}
			points := SamplePoints(parsed.StartedAt, stored)
			if len(points) == 0 {
				t.Fatal("no points")
			}
			coords := FlattenCoords(points)
			if metrics.HasGPS && len(coords) < 4 {
				t.Errorf("GPS activity produced only %d coordinates", len(coords))
			}

			t.Logf("samples=%d stored=%d sport=%s distance=%.0fm moving=%ds elevation=+%.0f/-%.0f hr=%v/%v coords=%d",
				len(parsed.Samples), len(stored), parsed.Sport, metrics.DistanceM,
				metrics.MovingTimeS, metrics.ElevationGainM, metrics.ElevationLossM,
				derefInt(metrics.AvgHeartRate), derefInt(metrics.MaxHeartRate), len(coords))
		})
	}

	if len(sportCounts) == 0 {
		t.Fatal("no files parsed")
	}
	t.Logf("sports found: %v", sportCounts)
}

// TestParseRealFITIsIdempotentInEncoding re-encodes a real file with our own
// writer and decodes it again: this is the export path exercised on real data.
func TestParseRealFITIsIdempotentInEncoding(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "..", "example", "*.fit"))
	if err != nil || len(matches) == 0 {
		t.Skip("no FIT files in example/")
	}
	path := matches[0]

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseFIT(file)
	_ = file.Close()
	if err != nil {
		t.Fatalf("ParseFIT: %v", err)
	}

	metrics := ComputeMetrics(parsed)
	stored := db.Activity{
		Sport: parsed.Sport, StartedAt: parsed.StartedAt.Unix(),
		ElapsedTimeS: metrics.ElapsedTimeS, MovingTimeS: metrics.MovingTimeS,
		DistanceM: metrics.DistanceM, ElevationGainM: metrics.ElevationGainM,
		ElevationLossM: metrics.ElevationLossM,
	}
	points := SamplePoints(parsed.StartedAt, Simplify(Decimate(parsed.Samples)))

	var buffer bytes.Buffer
	if err := WriteFIT(&buffer, stored, points); err != nil {
		t.Fatalf("WriteFIT: %v", err)
	}
	round, err := ParseFIT(bytes.NewReader(buffer.Bytes()))
	if err != nil {
		t.Fatalf("ParseFIT of our own output: %v", err)
	}

	roundMetrics := ComputeMetrics(round)
	if roundMetrics.ElapsedTimeS != metrics.ElapsedTimeS {
		t.Errorf("elapsed changed: %d -> %d", metrics.ElapsedTimeS, roundMetrics.ElapsedTimeS)
	}
	if roundMetrics.MovingTimeS != metrics.MovingTimeS {
		t.Errorf("moving changed: %d -> %d", metrics.MovingTimeS, roundMetrics.MovingTimeS)
	}
	if diff := roundMetrics.DistanceM - metrics.DistanceM; diff > 1 || diff < -1 {
		t.Errorf("distance changed: %.1f -> %.1f", metrics.DistanceM, roundMetrics.DistanceM)
	}
	t.Logf("round trip: %d points, elapsed=%ds moving=%ds distance=%.0fm",
		len(round.Samples), roundMetrics.ElapsedTimeS, roundMetrics.MovingTimeS, roundMetrics.DistanceM)
}

func derefInt(value *int) int {
	if value == nil {
		return -1
	}
	return *value
}
