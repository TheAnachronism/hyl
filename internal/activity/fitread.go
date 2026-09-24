package activity

import (
	"errors"
	"io"
	"math"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/proto"
)

// ErrNotAnActivity is returned for a valid FIT file that is not an activity.
var ErrNotAnActivity = errors.New("the FIT file does not contain an activity")

// ErrTooManyRecords is returned for a FIT file that expands into more records
// than hyl accepts.
var ErrTooManyRecords = errors.New("the FIT file contains more records than hyl accepts")

// maxFITRecords bounds how many record messages one upload may expand into. A
// minimal record message is about eleven bytes on the wire, so a 50 MiB upload
// could otherwise become millions of samples and gigabytes of heap. It is a
// variable only so tests can lower it instead of building a fixture with
// hundreds of thousands of records.
var maxFITRecords = 200_000

// ParseFIT decodes a FIT activity file. The decoder is put in broadcast-only
// mode and fed a listener, so it retains no messages: peak memory tracks the
// samples hyl keeps rather than every message in the file.
func ParseFIT(r io.Reader) (ParsedActivity, error) {
	collector := &fitCollector{}
	decoded, err := decoder.New(r,
		decoder.WithBroadcastOnly(),
		decoder.WithMesgListener(collector),
	).Decode()
	if err != nil {
		return ParsedActivity{}, err
	}
	if decoded.FileHeader.ProfileVersion == 0 {
		return ParsedActivity{}, errors.New("not a FIT file")
	}
	if collector.truncated {
		return ParsedActivity{}, ErrTooManyRecords
	}
	if !collector.sawFileID || collector.fileType != typedef.FileActivity {
		return ParsedActivity{}, ErrNotAnActivity
	}
	if len(collector.samples) == 0 {
		return ParsedActivity{}, ErrNotAnActivity
	}

	parsed := ParsedActivity{
		Samples:   collector.samples,
		StartedAt: collector.samples[0].T,
	}
	if session := collector.session; session != nil {
		parsed.Sport = fitSport(session.sport)
		if !session.startedAt.IsZero() {
			parsed.StartedAt = session.startedAt
		}
		parsed.SessionElapsedS = session.elapsedS
		parsed.SessionTimerS = session.timerS
		parsed.SessionDistanceM = session.distanceM
		parsed.SessionAscentM = session.ascentM
		parsed.SessionDescentM = session.descentM
		parsed.SessionAvgHR = session.avgHR
		parsed.SessionMaxHR = session.maxHR
	}
	if parsed.Sport == "" {
		parsed.Sport = SportOther
	}
	return parsed, nil
}

// fitCollector copies the few scalars hyl needs out of each decoded message.
// The decoder only guarantees a message's lifetime until OnMesg returns, so
// nothing that points into a message may be kept.
type fitCollector struct {
	sawFileID bool
	fileType  typedef.File
	session   *fitSession
	samples   []Sample
	truncated bool
}

// OnMesg implements decoder.MesgListener.
func (c *fitCollector) OnMesg(mesg proto.Message) {
	switch mesg.Num {
	case typedef.MesgNumFileId:
		if !c.sawFileID {
			fileID := mesgdef.NewFileId(&mesg)
			c.fileType = fileID.Type
			c.sawFileID = true
		}
	case typedef.MesgNumSession:
		if c.session == nil {
			c.session = newFitSession(mesgdef.NewSession(&mesg))
		}
	case typedef.MesgNumRecord:
		if len(c.samples) >= maxFITRecords {
			c.truncated = true
			return
		}
		record := mesgdef.NewRecord(&mesg)
		if record.Timestamp.IsZero() {
			return
		}
		c.samples = append(c.samples, recordSample(record))
	}
}

// fitSession is the session summary, already copied out of the decoder's
// message.
type fitSession struct {
	sport     typedef.Sport
	startedAt time.Time
	elapsedS  *int64
	timerS    *int64
	distanceM *float64
	ascentM   *float64
	descentM  *float64
	avgHR     *int
	maxHR     *int
}

// newFitSession copies the summary fields, scaling them the way the caller
// expects them.
func newFitSession(session *mesgdef.Session) *fitSession {
	out := &fitSession{sport: session.Sport, startedAt: session.StartTime}
	if value := session.TotalElapsedTimeScaled(); !math.IsNaN(value) {
		seconds := int64(math.Round(value))
		out.elapsedS = &seconds
	}
	if value := session.TotalTimerTimeScaled(); !math.IsNaN(value) {
		seconds := int64(math.Round(value))
		out.timerS = &seconds
	}
	if value := session.TotalDistanceScaled(); !math.IsNaN(value) {
		out.distanceM = &value
	}
	if session.TotalAscent != basetype.Uint16Invalid {
		ascent := float64(session.TotalAscent)
		out.ascentM = &ascent
	}
	if session.TotalDescent != basetype.Uint16Invalid {
		descent := float64(session.TotalDescent)
		out.descentM = &descent
	}
	if session.AvgHeartRate != basetype.Uint8Invalid {
		hr := int(session.AvgHeartRate)
		out.avgHR = &hr
	}
	if session.MaxHeartRate != basetype.Uint8Invalid {
		hr := int(session.MaxHeartRate)
		out.maxHR = &hr
	}
	return out
}

func recordSample(record *mesgdef.Record) Sample {
	sample := Sample{T: record.Timestamp.UTC()}

	if lat := record.PositionLatDegrees(); !math.IsNaN(lat) {
		sample.Lat = &lat
	}
	if lon := record.PositionLongDegrees(); !math.IsNaN(lon) {
		sample.Lon = &lon
	}
	// Enhanced altitude/speed are the modern fields; fall back when absent.
	if value := record.EnhancedAltitudeScaled(); !math.IsNaN(value) {
		sample.Ele = &value
	} else if value := record.AltitudeScaled(); !math.IsNaN(value) {
		sample.Ele = &value
	}
	if value := record.EnhancedSpeedScaled(); !math.IsNaN(value) {
		sample.Spd = &value
	} else if value := record.SpeedScaled(); !math.IsNaN(value) {
		sample.Spd = &value
	}
	if value := record.DistanceScaled(); !math.IsNaN(value) {
		sample.Dist = &value
	}
	if record.HeartRate != basetype.Uint8Invalid {
		hr := int(record.HeartRate)
		sample.HR = &hr
	}
	if record.Cadence != basetype.Uint8Invalid {
		cadence := int(record.Cadence)
		sample.Cad = &cadence
	}
	if record.Power != basetype.Uint16Invalid {
		power := int(record.Power)
		sample.Pwr = &power
	}
	return sample
}

// fitSport maps a FIT sport to a stored sport key. The FIT base-sport enum is
// coarse (trail and treadmill running are sub-sports of running), so every
// variant of a family maps to the same key.
func fitSport(sport typedef.Sport) string {
	switch sport {
	case typedef.SportRunning, typedef.SportWheelchairPushRun:
		return SportRun
	case typedef.SportCycling, typedef.SportEBiking:
		return SportRide
	case typedef.SportSwimming:
		return SportSwim
	case typedef.SportHiking, typedef.SportMountaineering:
		return SportHike
	case typedef.SportWalking, typedef.SportWheelchairPushWalk:
		return SportWalk
	case typedef.SportCrossCountrySkiing, typedef.SportAlpineSkiing, typedef.SportSnowboarding:
		return SportSki
	case typedef.SportRowing:
		return SportRow
	default:
		return SportOther
	}
}
