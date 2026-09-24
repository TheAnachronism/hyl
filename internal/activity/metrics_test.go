package activity

import (
	"math"
	"testing"
	"time"
)

var testStart = time.Date(2026, 1, 2, 8, 0, 0, 0, time.UTC)

func TestComputeMetricsElapsedAndMovingTime(t *testing.T) {
	// 600 s of riding, a 600 s pause, then 600 s more.
	var samples []Sample
	distance := 0.0
	add := func(offsetS int, speed float64) {
		sample := Sample{T: testStart.Add(time.Duration(offsetS) * time.Second)}
		speedCopy := speed
		distance += speed
		cumulative := distance
		sample.Spd = &speedCopy
		sample.Dist = &cumulative
		samples = append(samples, sample)
	}
	for second := 0; second <= 600; second++ {
		add(second, 5)
	}
	for second := 1201; second <= 1800; second++ {
		add(second, 5)
	}

	metrics := ComputeMetrics(ParsedActivity{StartedAt: testStart, Sport: SportRide, Samples: samples})

	if metrics.ElapsedTimeS != 1800 {
		t.Errorf("elapsed = %d, want 1800", metrics.ElapsedTimeS)
	}
	// 600 one-second segments before the pause plus 599 after it: the paused
	// segment itself is not counted.
	if metrics.MovingTimeS != 1199 {
		t.Errorf("moving = %d, want 1199 (the 600 s gap must not count)", metrics.MovingTimeS)
	}
	if metrics.DistanceM != 6005 {
		t.Errorf("distance = %.1f, want 6005 from the cumulative samples", metrics.DistanceM)
	}
	if metrics.AvgSpeedMps == nil || math.Abs(*metrics.AvgSpeedMps-5.0) > 0.01 {
		t.Errorf("avg speed = %v, want ~5 m/s", metrics.AvgSpeedMps)
	}
}

func TestComputeMetricsDistanceFallback(t *testing.T) {
	// No device totals and no cumulative distance: the haversine path is used.
	var samples []Sample
	for i := range 10 {
		lat := 52.0 + float64(i)*0.001
		lon := 5.0
		samples = append(samples, Sample{
			T:   testStart.Add(time.Duration(i) * time.Second),
			Lat: &lat,
			Lon: &lon,
		})
	}
	metrics := ComputeMetrics(ParsedActivity{StartedAt: testStart, Sport: SportWalk, Samples: samples})
	want := 9 * HaversineM(52.0, 5.0, 52.001, 5.0)
	if math.Abs(metrics.DistanceM-want) > 1.0 {
		t.Errorf("distance = %.1f, want ~%.1f", metrics.DistanceM, want)
	}
	if !metrics.HasGPS {
		t.Error("has_gps = false for a stream with coordinates")
	}
}

func TestComputeMetricsPrefersSessionTotals(t *testing.T) {
	var samples []Sample
	for i := range 5 {
		samples = append(samples, Sample{T: testStart.Add(time.Duration(i) * time.Second)})
	}
	elapsed, timer, distance := int64(900), int64(700), 12345.0
	metrics := ComputeMetrics(ParsedActivity{
		StartedAt: testStart, Sport: SportRide, Samples: samples,
		SessionElapsedS: &elapsed, SessionTimerS: &timer, SessionDistanceM: &distance,
	})
	if metrics.ElapsedTimeS != 900 || metrics.MovingTimeS != 700 || metrics.DistanceM != 12345 {
		t.Fatalf("session totals ignored: %+v", metrics)
	}
}

func TestElevationHysteresis(t *testing.T) {
	t.Run("noise wobble is ignored", func(t *testing.T) {
		var samples []Sample
		for i := range 60 {
			ele := 100.0
			if i%2 == 1 {
				ele = 100.6
			}
			samples = append(samples, Sample{T: testStart.Add(time.Duration(i) * time.Second), Ele: &ele})
		}
		gain, loss := elevationGainLoss(samples)
		if gain > 0.5 || loss > 0.5 {
			t.Errorf("noise produced gain=%.2f loss=%.2f, want both < 0.5", gain, loss)
		}
	})

	t.Run("a real climb counts", func(t *testing.T) {
		var samples []Sample
		for i := range 60 {
			ele := 100.0 + 12.0*float64(i)/59.0
			samples = append(samples, Sample{T: testStart.Add(time.Duration(i) * time.Second), Ele: &ele})
		}
		gain, loss := elevationGainLoss(samples)
		if gain < 11 || gain > 13 {
			t.Errorf("gain = %.2f, want ~12", gain)
		}
		if loss > 0.5 {
			t.Errorf("loss = %.2f, want ~0", loss)
		}
	})
}

func TestComputeMetricsOptionalSeries(t *testing.T) {
	var samples []Sample
	for i := range 10 {
		sample := Sample{T: testStart.Add(time.Duration(i) * time.Second)}
		if i%2 == 0 {
			hr := 120 + i
			sample.HR = &hr
			cad := 80 + i
			sample.Cad = &cad
			power := 200 + i
			sample.Pwr = &power
		}
		samples = append(samples, sample)
	}
	metrics := ComputeMetrics(ParsedActivity{StartedAt: testStart, Sport: SportRide, Samples: samples})
	if metrics.AvgHeartRate == nil || *metrics.AvgHeartRate != 124 {
		t.Errorf("avg hr = %v, want 124", metrics.AvgHeartRate)
	}
	if metrics.MaxHeartRate == nil || *metrics.MaxHeartRate != 128 {
		t.Errorf("max hr = %v, want 128", metrics.MaxHeartRate)
	}
	if metrics.AvgCadence == nil || *metrics.AvgCadence != 84 {
		t.Errorf("avg cadence = %v, want 84", metrics.AvgCadence)
	}
	if metrics.AvgPowerW == nil || *metrics.AvgPowerW != 204 {
		t.Errorf("avg power = %v, want 204", metrics.AvgPowerW)
	}

	// A stream with no heart rate reports nothing rather than zero.
	var bare []Sample
	for i := range 5 {
		bare = append(bare, Sample{T: testStart.Add(time.Duration(i) * time.Second)})
	}
	metrics = ComputeMetrics(ParsedActivity{StartedAt: testStart, Sport: SportRide, Samples: bare})
	if metrics.AvgHeartRate != nil || metrics.MaxHeartRate != nil {
		t.Errorf("missing heart rate reported as %v/%v", metrics.AvgHeartRate, metrics.MaxHeartRate)
	}
	if metrics.HasGPS {
		t.Error("has_gps = true without coordinates")
	}
}

func TestComputeMetricsGuardsZeroMovingTime(t *testing.T) {
	// A single point: no elapsed span, so the average speed must stay nil.
	lat, lon := 52.0, 5.0
	samples := []Sample{{T: testStart, Lat: &lat, Lon: &lon}}
	metrics := ComputeMetrics(ParsedActivity{StartedAt: testStart, Sport: SportRide, Samples: samples})
	if metrics.ElapsedTimeS != 0 || metrics.MovingTimeS != 0 {
		t.Errorf("elapsed/moving = %d/%d, want 0/0", metrics.ElapsedTimeS, metrics.MovingTimeS)
	}
	if metrics.AvgSpeedMps != nil {
		t.Errorf("avg speed = %v, want nil", *metrics.AvgSpeedMps)
	}
}

func TestHaversineM(t *testing.T) {
	// One degree of latitude is about 111 km.
	distance := HaversineM(52.0, 5.0, 53.0, 5.0)
	if math.Abs(distance-111195) > 500 {
		t.Errorf("haversine = %.0f, want ~111195", distance)
	}
	if HaversineM(52.0, 5.0, 52.0, 5.0) != 0 {
		t.Error("haversine of identical points is not zero")
	}
}
