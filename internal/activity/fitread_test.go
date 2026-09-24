package activity

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/markbeep/hyl/internal/db"
)

// fitWithRecords builds a valid FIT activity with the given number of points,
// which is the cheapest way to exercise the record cap.
func fitWithRecords(t *testing.T, records int) []byte {
	t.Helper()
	start := time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC)
	stored := db.Activity{
		Sport: SportRide, StartedAt: start.Unix(),
		ElapsedTimeS: int64(records), MovingTimeS: int64(records), DistanceM: float64(records),
	}
	points := make([]Point, 0, records)
	for i := range records {
		latitude := 52.0 + float64(i)*0.0001
		longitude := 5.0
		distance := float64(i)
		points = append(points, Point{
			Seq: int64(i), T: start.Add(time.Duration(i) * time.Second).Unix(),
			ElapsedS: int64(i), Lat: &latitude, Lon: &longitude, DistM: &distance,
		})
	}
	var buffer bytes.Buffer
	if err := WriteFIT(&buffer, stored, points); err != nil {
		t.Fatalf("WriteFIT: %v", err)
	}
	return buffer.Bytes()
}

// TestParseFITRejectsTooManyRecords pins the memory guard: a file that expands
// past the record budget is refused rather than turned into an unbounded slice.
func TestParseFITRejectsTooManyRecords(t *testing.T) {
	payload := fitWithRecords(t, 40)

	original := maxFITRecords
	maxFITRecords = 10
	t.Cleanup(func() { maxFITRecords = original })

	if _, err := ParseFIT(bytes.NewReader(payload)); !errors.Is(err, ErrTooManyRecords) {
		t.Fatalf("ParseFIT error = %v, want ErrTooManyRecords", err)
	}

	// The same file is fine one record under the budget, so the guard is a cap
	// and not a blanket rejection.
	maxFITRecords = 40
	parsed, err := ParseFIT(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("ParseFIT under the cap: %v", err)
	}
	if len(parsed.Samples) != 40 {
		t.Fatalf("samples = %d, want 40", len(parsed.Samples))
	}
}
