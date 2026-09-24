package activity

import "math"

// MovingSpeedThresholdMps is 1.0 km/h: below it a segment counts as stopped.
const MovingSpeedThresholdMps = 0.2778

// maxMovingGapS bounds the gap between two samples that can still count as
// moving; anything longer is a pause.
const maxMovingGapS = 30.0

// earthRadiusM is the mean earth radius used by every haversine in the package.
const earthRadiusM = 6371008.8

// Metrics are the numeric columns stored on an activity.
type Metrics struct {
	ElapsedTimeS   int64
	MovingTimeS    int64
	DistanceM      float64
	ElevationGainM float64
	ElevationLossM float64
	AvgSpeedMps    *float64
	MaxSpeedMps    *float64
	AvgHeartRate   *int
	MaxHeartRate   *int
	AvgCadence     *float64
	MaxCadence     *int
	AvgPowerW      *float64
	MaxPowerW      *int
	HasGPS         bool
}

// ComputeMetrics derives every displayed number from a parsed activity.
//
// The order of precedence is fixed and matches the documented contract:
// device session totals win over anything computed from the stream, and every
// computed fallback degrades gracefully when the file carries less data.
func ComputeMetrics(p ParsedActivity) Metrics {
	var m Metrics
	samples := p.Samples
	if len(samples) == 0 {
		if p.SessionElapsedS != nil {
			m.ElapsedTimeS = *p.SessionElapsedS
		}
		if p.SessionTimerS != nil {
			m.MovingTimeS = *p.SessionTimerS
		}
		if p.SessionDistanceM != nil && *p.SessionDistanceM > 0 {
			m.DistanceM = *p.SessionDistanceM
		}
		if p.SessionAscentM != nil {
			m.ElevationGainM = *p.SessionAscentM
		}
		if p.SessionDescentM != nil {
			m.ElevationLossM = *p.SessionDescentM
		}
		m.AvgHeartRate = p.SessionAvgHR
		m.MaxHeartRate = p.SessionMaxHR
		return m
	}

	first, last := samples[0], samples[len(samples)-1]

	// Elapsed time.
	if p.SessionElapsedS != nil {
		m.ElapsedTimeS = *p.SessionElapsedS
	} else {
		m.ElapsedTimeS = int64(last.T.Sub(first.T).Seconds())
	}

	// Moving time: session timer, else the sum of moving segments.
	if p.SessionTimerS != nil {
		m.MovingTimeS = *p.SessionTimerS
	} else {
		m.MovingTimeS = movingTimeS(samples)
	}

	// Distance: device total, else the last cumulative sample, else haversine.
	switch {
	case p.SessionDistanceM != nil && *p.SessionDistanceM > 0:
		m.DistanceM = *p.SessionDistanceM
	default:
		if d := lastCumulativeDistance(samples); d > 0 {
			m.DistanceM = d
		} else {
			m.DistanceM = haversinePathM(samples)
		}
	}

	// Elevation: device totals, else a smoothed, hysteresis-filtered climb.
	if p.SessionAscentM != nil && p.SessionDescentM != nil {
		m.ElevationGainM = *p.SessionAscentM
		m.ElevationLossM = *p.SessionDescentM
	} else {
		m.ElevationGainM, m.ElevationLossM = elevationGainLoss(samples)
	}

	if m.MovingTimeS > 0 {
		avg := m.DistanceM / float64(m.MovingTimeS)
		m.AvgSpeedMps = &avg
	}
	if max := maxFloat(samples, func(s Sample) *float64 { return s.Spd }); max != nil {
		m.MaxSpeedMps = max
	}

	moving := movingSamples(samples)
	if avg, ok := meanInt(moving, func(s Sample) *int { return s.HR }); ok {
		m.AvgHeartRate = &avg
	}
	if max, ok := maxInt(samples, func(s Sample) *int { return s.HR }); ok {
		m.MaxHeartRate = &max
	}
	if avg, ok := meanInt(moving, func(s Sample) *int { return s.Cad }); ok {
		value := float64(avg)
		m.AvgCadence = &value
	}
	if max, ok := maxInt(samples, func(s Sample) *int { return s.Cad }); ok {
		m.MaxCadence = &max
	}
	if avg, ok := meanInt(moving, func(s Sample) *int { return s.Pwr }); ok {
		value := float64(avg)
		m.AvgPowerW = &value
	}
	if max, ok := maxInt(samples, func(s Sample) *int { return s.Pwr }); ok {
		m.MaxPowerW = &max
	}

	withCoords := 0
	for _, sample := range samples {
		if sample.Lat != nil && sample.Lon != nil {
			withCoords++
		}
	}
	m.HasGPS = withCoords >= 2

	return m
}

// movingTimeS sums the segments that count as moving: at most 30 s long and
// covering at least 1 km/h by speed, distance delta or haversine.
func movingTimeS(samples []Sample) int64 {
	var total float64
	for i := 1; i < len(samples); i++ {
		prev, cur := samples[i-1], samples[i]
		dt := cur.T.Sub(prev.T).Seconds()
		if dt <= 0 || dt > maxMovingGapS {
			continue
		}
		if segmentMoving(prev, cur, dt) {
			total += dt
		}
	}
	return int64(total)
}

func segmentMoving(prev, cur Sample, dt float64) bool {
	if cur.Spd != nil {
		return *cur.Spd >= MovingSpeedThresholdMps
	}
	if prev.Dist != nil && cur.Dist != nil {
		return (*cur.Dist-*prev.Dist)/dt >= MovingSpeedThresholdMps
	}
	if prev.Lat != nil && prev.Lon != nil && cur.Lat != nil && cur.Lon != nil {
		return HaversineM(*prev.Lat, *prev.Lon, *cur.Lat, *cur.Lon)/dt >= MovingSpeedThresholdMps
	}
	return false
}

// movingSamples returns the samples that belong to a moving segment, so
// averages are not diluted by rest stops.
func movingSamples(samples []Sample) []Sample {
	if len(samples) < 2 {
		return nil
	}
	out := make([]Sample, 0, len(samples))
	for i := 1; i < len(samples); i++ {
		prev, cur := samples[i-1], samples[i]
		dt := cur.T.Sub(prev.T).Seconds()
		if dt > 0 && dt <= maxMovingGapS && segmentMoving(prev, cur, dt) {
			out = append(out, cur)
		}
	}
	if len(out) == 0 {
		return samples
	}
	return out
}

func lastCumulativeDistance(samples []Sample) float64 {
	for i := len(samples) - 1; i >= 0; i-- {
		if samples[i].Dist != nil {
			return *samples[i].Dist
		}
	}
	return 0
}

func haversinePathM(samples []Sample) float64 {
	var total float64
	for i := 1; i < len(samples); i++ {
		prev, cur := samples[i-1], samples[i]
		if prev.Lat == nil || prev.Lon == nil || cur.Lat == nil || cur.Lon == nil {
			continue
		}
		total += HaversineM(*prev.Lat, *prev.Lon, *cur.Lat, *cur.Lon)
	}
	return total
}

// elevationGainLoss smooths the altitude series with a centred 5-sample moving
// average, then accumulates runs. A reversal only starts a new run once the
// smoothed altitude has moved more than 1 m against the direction of travel, so
// GPS noise does not inflate the climb.
func elevationGainLoss(samples []Sample) (gain, loss float64) {
	raw := make([]float64, 0, len(samples))
	for _, sample := range samples {
		if sample.Ele != nil {
			raw = append(raw, *sample.Ele)
		}
	}
	if len(raw) < 2 {
		return 0, 0
	}

	smoothed := make([]float64, len(raw))
	for i := range raw {
		lo := max(0, i-2)
		hi := min(len(raw)-1, i+2)
		var sum float64
		for j := lo; j <= hi; j++ {
			sum += raw[j]
		}
		smoothed[i] = sum / float64(hi-lo+1)
	}

	const hysteresis = 1.0

	// ref is the last confirmed turning point, extreme the best value reached
	// since then, hi/lo the undecided extremes while no direction is set.
	ref := smoothed[0]
	extreme := smoothed[0]
	hi, lo := smoothed[0], smoothed[0]
	dir := 0

	for _, value := range smoothed[1:] {
		switch dir {
		case 0:
			hi = math.Max(hi, value)
			lo = math.Min(lo, value)
			if hi-lo <= hysteresis {
				continue
			}
			if hi == value { // the climb started at lo
				loss += math.Max(0, ref-lo)
				ref, dir = lo, 1
				extreme = value
			} else { // the descent started at hi
				gain += math.Max(0, hi-ref)
				ref, dir = hi, -1
				extreme = value
			}
		case 1:
			if value > extreme {
				extreme = value
			} else if extreme-value > hysteresis {
				gain += extreme - ref
				ref, dir = extreme, -1
				extreme = value
			}
		case -1:
			if value < extreme {
				extreme = value
			} else if value-extreme > hysteresis {
				loss += ref - extreme
				ref, dir = extreme, 1
				extreme = value
			}
		}
	}

	switch dir {
	case 1:
		gain += math.Max(0, extreme-ref)
	case -1:
		loss += math.Max(0, ref-extreme)
	}
	return math.Max(0, gain), math.Max(0, loss)
}

// HaversineM is the great-circle distance in metres between two positions.
func HaversineM(lat1, lon1, lat2, lon2 float64) float64 {
	const degToRad = math.Pi / 180
	phi1 := lat1 * degToRad
	phi2 := lat2 * degToRad
	dPhi := (lat2 - lat1) * degToRad
	dLambda := (lon2 - lon1) * degToRad
	a := math.Sin(dPhi/2)*math.Sin(dPhi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(dLambda/2)*math.Sin(dLambda/2)
	return 2 * earthRadiusM * math.Asin(math.Min(1, math.Sqrt(a)))
}

func maxFloat(samples []Sample, pick func(Sample) *float64) *float64 {
	var best *float64
	for _, sample := range samples {
		value := pick(sample)
		if value == nil {
			continue
		}
		if best == nil || *value > *best {
			v := *value
			best = &v
		}
	}
	return best
}

func meanInt(samples []Sample, pick func(Sample) *int) (int, bool) {
	var sum, count int
	for _, sample := range samples {
		if value := pick(sample); value != nil {
			sum += *value
			count++
		}
	}
	if count == 0 {
		return 0, false
	}
	return int(math.Round(float64(sum) / float64(count))), true
}

func maxInt(samples []Sample, pick func(Sample) *int) (int, bool) {
	var best int
	found := false
	for _, sample := range samples {
		if value := pick(sample); value != nil && (!found || *value > best) {
			best = *value
			found = true
		}
	}
	return best, found
}
