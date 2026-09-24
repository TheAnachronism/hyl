package dto

import (
	"github.com/markbeep/hyl/internal/api"
	"github.com/markbeep/hyl/internal/db"
)

// ActivitySummary renders one activity list row. track is the already
// privacy-trimmed coordinate list; photos is the (possibly truncated) photo
// strip.
func ActivitySummary(row db.ListActivitiesRow, track []float64, mapAvailable bool, photos []api.Photo) api.ActivitySummary {
	if track == nil {
		// A hidden route is an empty list, never null, so clients can always
		// treat the field as an array.
		track = []float64{}
	}
	if photos == nil {
		photos = []api.Photo{}
	}
	return api.ActivitySummary{
		ID:             row.ID,
		UserID:         row.UserID,
		Username:       row.Username,
		DisplayName:    row.DisplayName,
		AvatarURL:      AvatarURL(row.AvatarMediaID),
		Title:          row.Title,
		Description:    row.Description,
		Sport:          row.Sport,
		StartedAt:      api.Timestamp(row.StartedAt),
		DistanceM:      row.DistanceM,
		MovingTimeS:    row.MovingTimeS,
		ElapsedTimeS:   row.ElapsedTimeS,
		ElevationGainM: row.ElevationGainM,
		AvgSpeedMps:    row.AvgSpeedMps,
		MaxSpeedMps:    row.MaxSpeedMps,
		AvgHeartRate:   row.AvgHeartRate,
		MaxHeartRate:   row.MaxHeartRate,
		AvgCadence:     row.AvgCadence,
		AvgPowerW:      row.AvgPowerW,
		LikeCount:      row.LikeCount,
		CommentCount:   row.CommentCount,
		LikedByMe:      row.LikedByMe,
		PhotoCount:     row.PhotoCount,
		Visibility:     row.Visibility,
		HasGps:         row.HasGps,
		RouteHidden:    row.RouteHidden,
		Track:          track,
		MapAvailable:   mapAvailable,
		Photos:         photos,
	}
}

// ActivityPage assembles one page-numbered activity list. items is never null.
func ActivityPage(items []api.ActivitySummary, page, pageSize, total, totalPages int64) api.ActivityPage {
	if items == nil {
		items = []api.ActivitySummary{}
	}
	return api.ActivityPage{
		Items:      items,
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	}
}

// ActivityCounts are the per-activity aggregates every row carries.
type ActivityCounts struct {
	LikeCount    int64
	CommentCount int64
	PhotoCount   int64
	LikedByMe    bool
}

// ActivitySummaryFromActivity renders one activity for the detail endpoint,
// where the row comes from the activities table rather than the list query.
func ActivitySummaryFromActivity(a db.Activity, owner db.User, avatarMediaID int64, counts ActivityCounts, track []float64, mapAvailable bool, photos []api.Photo) api.ActivitySummary {
	if track == nil {
		track = []float64{}
	}
	if photos == nil {
		photos = []api.Photo{}
	}
	return api.ActivitySummary{
		ID:             a.ID,
		UserID:         a.UserID,
		Username:       owner.Username,
		DisplayName:    owner.DisplayName,
		AvatarURL:      AvatarURL(avatarMediaID),
		Title:          a.Title,
		Description:    a.Description,
		Sport:          a.Sport,
		StartedAt:      api.Timestamp(a.StartedAt),
		DistanceM:      a.DistanceM,
		MovingTimeS:    a.MovingTimeS,
		ElapsedTimeS:   a.ElapsedTimeS,
		ElevationGainM: a.ElevationGainM,
		AvgSpeedMps:    a.AvgSpeedMps,
		MaxSpeedMps:    a.MaxSpeedMps,
		AvgHeartRate:   a.AvgHeartRate,
		MaxHeartRate:   a.MaxHeartRate,
		AvgCadence:     a.AvgCadence,
		AvgPowerW:      a.AvgPowerW,
		LikeCount:      counts.LikeCount,
		CommentCount:   counts.CommentCount,
		LikedByMe:      counts.LikedByMe,
		PhotoCount:     counts.PhotoCount,
		Visibility:     a.Visibility,
		HasGps:         a.HasGps,
		RouteHidden:    a.RouteHidden,
		Track:          track,
		MapAvailable:   mapAvailable,
		Photos:         photos,
	}
}

// ActivityDetail builds the single-activity payload from its summary.
func ActivityDetail(summary api.ActivitySummary, route []float64, streams api.ActivityStreams, canEdit bool) api.ActivityDetail {
	if summary.Photos == nil {
		summary.Photos = []api.Photo{}
	}
	if route == nil {
		route = []float64{}
	}
	if streams.ElapsedS == nil {
		streams = api.ActivityStreams{
			ElapsedS: []int64{}, HR: []*int64{}, Cadence: []*int64{},
			Power: []*int64{}, SpeedMps: []*float64{}, ElevationM: []*float64{},
		}
	}
	return api.ActivityDetail{
		ActivitySummary: summary,
		CanEdit:         canEdit,
		Route:           route,
		Streams:         streams,
	}
}
