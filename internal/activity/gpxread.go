package activity

import (
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/tkrajina/gpxgo/gpx"
)

// ErrNotAGPX is returned when a document carries no track points.
var ErrNotAGPX = errors.New("the GPX file contains no track points")

// ParseGPX decodes a GPX track. GPX carries neither a sport nor session
// totals, so the sport stays empty here and the caller applies the upload's
// sport field.
func ParseGPX(r io.Reader) (ParsedActivity, error) {
	document, err := gpx.Parse(r)
	if err != nil {
		return ParsedActivity{}, err
	}

	parsed := ParsedActivity{}
	for _, track := range document.Tracks {
		for _, segment := range track.Segments {
			for i := range segment.Points {
				point := segment.Points[i]
				if point.Timestamp.IsZero() {
					continue
				}
				sample := Sample{T: point.Timestamp.UTC()}
				lat := point.Latitude
				lon := point.Longitude
				sample.Lat = &lat
				sample.Lon = &lon
				if point.Elevation.NotNull() {
					ele := point.Elevation.Value()
					sample.Ele = &ele
				}
				applyTrackPointExtensions(&point, &sample)
				parsed.Samples = append(parsed.Samples, sample)
			}
		}
	}
	if len(parsed.Samples) == 0 {
		return ParsedActivity{}, ErrNotAGPX
	}
	parsed.StartedAt = parsed.Samples[0].T
	return parsed, nil
}

// applyTrackPointExtensions reads the Garmin gpxtpx heart rate and cadence.
func applyTrackPointExtensions(point *gpx.GPXPoint, sample *Sample) {
	extension, ok := point.Extensions.GetNode(gpx.AnyNamespace, "TrackPointExtension")
	if !ok {
		return
	}
	if node, found := extension.GetNode("hr"); found {
		if value, err := strconv.Atoi(strings.TrimSpace(node.Data)); err == nil {
			sample.HR = &value
		}
	}
	if node, found := extension.GetNode("cad"); found {
		if value, err := strconv.Atoi(strings.TrimSpace(node.Data)); err == nil {
			sample.Cad = &value
		}
	}
}
