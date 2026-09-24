package activity

import (
	"bytes"
	"math"
	"os"
	"testing"
	"time"

	"github.com/markbeep/hyl/internal/db"
)

// TestWriteFITRoundTrip is the strongest available check of the exporter: the
// bytes it produces are decoded by our own reader, so a wrong scale, a wrong
// epoch or a dropped field shows up immediately.
func TestWriteFITRoundTrip(t *testing.T) {
	start := time.Date(2026, 3, 4, 6, 30, 0, 0, time.UTC)
	activity := db.Activity{
		ID:             7,
		UserID:         1,
		Title:          "Interval ride",
		Sport:          SportRide,
		StartedAt:      start.Unix(),
		ElapsedTimeS:   600,
		MovingTimeS:    540,
		DistanceM:      5000.5,
		ElevationGainM: 120,
		ElevationLossM: 118,
		AvgSpeedMps:    ptrFloat(9.26),
		MaxSpeedMps:    ptrFloat(14.2),
		AvgHeartRate:   ptrInt64(142),
		MaxHeartRate:   ptrInt64(178),
		AvgCadence:     ptrFloat(88),
		AvgPowerW:      ptrFloat(210),
	}

	var points []Point
	for i := range 60 {
		lat := 52.0 + float64(i)*0.0002
		lon := 5.0 + float64(i)*0.0001
		ele := 10.0 + float64(i)*0.5
		speed := 8.0 + float64(i%5)
		distance := float64(i) * 83.3
		hr := 130 + i%10
		cadence := 85 + i%3
		power := 200 + i%20
		points = append(points, Point{
			Seq:      int64(i),
			T:        start.Add(time.Duration(i*10) * time.Second).Unix(),
			ElapsedS: int64(i * 10),
			Lat:      &lat,
			Lon:      &lon,
			Ele:      &ele,
			HR:       &hr,
			Cad:      &cadence,
			Pwr:      &power,
			Spd:      &speed,
			DistM:    &distance,
		})
	}

	var buffer bytes.Buffer
	if err := WriteFIT(&buffer, activity, points); err != nil {
		t.Fatalf("WriteFIT: %v", err)
	}
	if buffer.Len() == 0 {
		t.Fatal("WriteFIT wrote nothing")
	}

	parsed, err := ParseFIT(bytes.NewReader(buffer.Bytes()))
	if err != nil {
		t.Fatalf("ParseFIT of our own output: %v", err)
	}
	if len(parsed.Samples) != len(points) {
		t.Fatalf("round trip produced %d samples, want %d", len(parsed.Samples), len(points))
	}
	if parsed.Sport != SportRide {
		t.Fatalf("sport = %q, want ride", parsed.Sport)
	}
	if !parsed.StartedAt.Equal(start) {
		t.Fatalf("start = %s, want %s", parsed.StartedAt, start)
	}
	if got := parsed.Samples[len(parsed.Samples)-1].T; !got.Equal(start.Add(590 * time.Second)) {
		t.Fatalf("last timestamp = %s", got)
	}

	first := parsed.Samples[0]
	if first.Lat == nil || math.Abs(*first.Lat-52.0) > 1e-5 {
		t.Fatalf("latitude lost: %v", first.Lat)
	}
	if first.Ele == nil || math.Abs(*first.Ele-10.0) > 0.3 {
		t.Fatalf("altitude lost: %v", first.Ele)
	}
	if first.HR == nil || *first.HR != 130 {
		t.Fatalf("heart rate lost: %v", first.HR)
	}
	if first.Cad == nil || *first.Cad != 85 {
		t.Fatalf("cadence lost: %v", first.Cad)
	}
	if first.Pwr == nil || *first.Pwr != 200 {
		t.Fatalf("power lost: %v", first.Pwr)
	}

	metrics := ComputeMetrics(parsed)
	if math.Abs(metrics.DistanceM-5000.5) > 1.0 {
		t.Fatalf("session distance = %.1f, want 5000.5", metrics.DistanceM)
	}
	if metrics.ElapsedTimeS != 600 || metrics.MovingTimeS != 540 {
		t.Fatalf("session times = %d/%d, want 600/540", metrics.ElapsedTimeS, metrics.MovingTimeS)
	}
	if parsed.SessionAvgHR == nil || *parsed.SessionAvgHR != 142 {
		t.Fatalf("session avg hr = %v, want 142", parsed.SessionAvgHR)
	}
	// The stream carries its own heart rates, which are what the computed
	// metrics use.
	if metrics.AvgHeartRate == nil || *metrics.AvgHeartRate < 130 || *metrics.AvgHeartRate > 139 {
		t.Fatalf("computed avg hr = %v, want 130-139", metrics.AvgHeartRate)
	}
}

func TestWriteFITInterpolatesDroppedCoordinates(t *testing.T) {
	start := time.Date(2026, 3, 4, 6, 30, 0, 0, time.UTC)
	activity := db.Activity{
		Sport: SportRun, StartedAt: start.Unix(), ElapsedTimeS: 40, MovingTimeS: 40, DistanceM: 400,
	}
	var points []Point
	for i := range 5 {
		lat := 52.0 + float64(i)*0.001
		lon := 5.0
		point := Point{Seq: int64(i), T: start.Add(time.Duration(i*10) * time.Second).Unix(), ElapsedS: int64(i * 10)}
		// The middle points lost their coordinates to simplification.
		if i != 0 && i != 4 {
			points = append(points, point)
			continue
		}
		point.Lat, point.Lon = &lat, &lon
		points = append(points, point)
	}

	var buffer bytes.Buffer
	if err := WriteFIT(&buffer, activity, points); err != nil {
		t.Fatalf("WriteFIT: %v", err)
	}
	parsed, err := ParseFIT(bytes.NewReader(buffer.Bytes()))
	if err != nil {
		t.Fatalf("ParseFIT: %v", err)
	}
	if len(parsed.Samples) != 5 {
		t.Fatalf("got %d samples, want 5", len(parsed.Samples))
	}
	for i, sample := range parsed.Samples {
		if sample.Lat == nil {
			t.Fatalf("sample %d has no position", i)
		}
		want := 52.0 + float64(i)*0.001
		if math.Abs(*sample.Lat-want) > 1e-5 {
			t.Fatalf("sample %d latitude = %.6f, want %.6f", i, *sample.Lat, want)
		}
	}
}

func TestWriteFITOmitPointsOutsideAnchors(t *testing.T) {
	start := time.Date(2026, 3, 4, 6, 30, 0, 0, time.UTC)
	activity := db.Activity{Sport: SportRun, StartedAt: start.Unix(), ElapsedTimeS: 60, DistanceM: 100}
	var points []Point
	for i := range 4 {
		point := Point{Seq: int64(i), T: start.Add(time.Duration(i*10) * time.Second).Unix()}
		if i == 1 || i == 2 {
			lat, lon := 52.0+float64(i)*0.001, 5.0
			point.Lat, point.Lon = &lat, &lon
		}
		points = append(points, point)
	}

	var buffer bytes.Buffer
	if err := WriteFIT(&buffer, activity, points); err != nil {
		t.Fatalf("WriteFIT: %v", err)
	}
	parsed, err := ParseFIT(bytes.NewReader(buffer.Bytes()))
	if err != nil {
		t.Fatalf("ParseFIT: %v", err)
	}
	if len(parsed.Samples) != 2 {
		t.Fatalf("got %d samples, want the 2 anchor points only", len(parsed.Samples))
	}
}

func TestLooksLikeFIT(t *testing.T) {
	if looksLikeFIT([]byte("<?xml version=\"1.0\"?><gpx>")) {
		t.Error("GPX was detected as FIT")
	}
	header := make([]byte, 14)
	header[0] = 12
	copy(header[8:12], ".FIT")
	if !looksLikeFIT(header) {
		t.Error("a FIT header was not detected")
	}
}

func ptrFloat(value float64) *float64 { return &value }
func ptrInt64(value int64) *int64     { return &value }

// TestParseGPXFixture parses the checked-in fixture the manual verification
// steps upload.
func TestParseGPXFixture(t *testing.T) {
	file, err := os.Open("testdata/ride.gpx")
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	defer func() { _ = file.Close() }()

	parsed, err := ParseGPX(file)
	if err != nil {
		t.Fatalf("ParseGPX: %v", err)
	}
	if len(parsed.Samples) != 60 {
		t.Fatalf("parsed %d samples, want 60", len(parsed.Samples))
	}
	first := parsed.Samples[0]
	if first.HR == nil || *first.HR != 138 {
		t.Fatalf("heart rate extension lost: %v", first.HR)
	}
	if first.Cad == nil || *first.Cad != 84 {
		t.Fatalf("cadence extension lost: %v", first.Cad)
	}
	if first.Ele == nil || *first.Ele < 11 || *first.Ele > 13 {
		t.Fatalf("elevation lost: %v", first.Ele)
	}
	if parsed.Sport != "" {
		t.Fatalf("GPX reported sport %q, want empty so the upload field wins", parsed.Sport)
	}

	metrics := ComputeMetrics(parsed)
	if metrics.DistanceM < 1500 || metrics.DistanceM > 3000 {
		t.Fatalf("fixture distance = %.0f m, want 1.5-3 km", metrics.DistanceM)
	}
	if metrics.ElapsedTimeS != 590 {
		t.Fatalf("elapsed = %d, want 590", metrics.ElapsedTimeS)
	}
}

func TestParsePayloadSniffsFormats(t *testing.T) {
	gpxBytes, err := os.ReadFile("testdata/ride.gpx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePayload(gpxBytes); err != nil {
		t.Fatalf("plain GPX: %v", err)
	}
	gzipped, err := os.ReadFile("testdata/ride.gpx.gz")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePayload(gzipped); err != nil {
		t.Fatalf("gzipped GPX: %v", err)
	}
	if _, err := ParsePayload([]byte("not an activity at all")); err == nil {
		t.Fatal("garbage was accepted as an activity")
	}
}
