package activity

import (
	"context"
	"database/sql"
	"errors"

	"github.com/markbeep/hyl/internal/apperr"
	"github.com/markbeep/hyl/internal/db"
	"github.com/markbeep/hyl/internal/social"
)

// ViewerActivity is an activity the viewer may open. The map coordinates are
// already trimmed the way the activity page trims them. A missing activity and
// an activity this viewer cannot see are both reported as not found.
type ViewerActivity struct {
	Activity     db.Activity
	Owner        db.User
	Points       []Point
	Track        []float64
	Route        []float64
	MapAvailable bool
}

// VisibleActivity reports whether the viewer may open the activity. It loads
// the row, the owner and an accepted follow, and nothing else: no point stream
// and no privacy trim. Viewer id zero is anonymous. A missing activity and an
// activity this viewer cannot see are both not found.
func (h *Handlers) VisibleActivity(ctx context.Context, activityID, viewerID int64) (db.Activity, error) {
	row, _, err := h.visible(ctx, activityID, viewerID)
	return row, err
}

// ForViewer loads one activity for a viewer. Visibility is VisibleActivity.
// The route is trimmed once, then decimated to the list and detail budgets.
func (h *Handlers) ForViewer(ctx context.Context, activityID, viewerID int64) (ViewerActivity, error) {
	row, owner, err := h.visible(ctx, activityID, viewerID)
	if err != nil {
		return ViewerActivity{}, err
	}

	points, err := h.points(ctx, activityID)
	if err != nil {
		return ViewerActivity{}, err
	}
	zones, err := newZoneCache(ctx, h.Q).forOwner(owner.ID, owner.TrimScope)
	if err != nil {
		return ViewerActivity{}, err
	}
	track, route, mapAvailable := trimmedDisplayMaps(points, row.RouteHidden, zones, owner.TrimScope,
		float64(owner.TrimRadiusM), row.DistanceM)
	return ViewerActivity{
		Activity: row, Owner: owner, Points: points,
		Track: track, Route: route, MapAvailable: mapAvailable,
	}, nil
}

func (h *Handlers) visible(ctx context.Context, activityID, viewerID int64) (db.Activity, db.User, error) {
	row, err := h.Q.GetActivity(ctx, activityID)
	if errors.Is(err, sql.ErrNoRows) {
		return db.Activity{}, db.User{}, apperr.NotFound("no such activity")
	}
	if err != nil {
		return db.Activity{}, db.User{}, err
	}
	owner, err := h.Q.GetUserByID(ctx, row.UserID)
	if err != nil {
		return db.Activity{}, db.User{}, err
	}

	follower := false
	if viewerID != 0 && viewerID != owner.ID {
		follow, err := h.Q.GetFollow(ctx, viewerID, owner.ID)
		switch {
		case err == nil:
			follower = follow.Status == "accepted"
		case errors.Is(err, sql.ErrNoRows):
		default:
			return db.Activity{}, db.User{}, err
		}
	}
	if !social.VisibilityAllows(viewerID, owner.ID, follower, row.Visibility, owner.ActivitiesVisibility) {
		return db.Activity{}, db.User{}, apperr.NotFound("no such activity")
	}
	return row, owner, nil
}
