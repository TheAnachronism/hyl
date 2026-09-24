package social

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/markbeep/hyl/internal/api"
	"github.com/markbeep/hyl/internal/apperr"
	"github.com/markbeep/hyl/internal/db"
	"github.com/markbeep/hyl/internal/dto"
	"github.com/markbeep/hyl/internal/reqctx"
)

// Like records a like. There is deliberately no unlike endpoint: a like is a
// one-way signal, and repeating the call is idempotent.
func (h *Handlers) Like(c echo.Context) error {
	viewer, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	activityID, err := dto.IDParam(c, "id")
	if err != nil {
		return err
	}
	ctx := c.Request().Context()

	activity, err := h.visibleActivity(ctx, activityID, viewer.ID)
	if err != nil {
		return err
	}
	// Peaking your own activity is refused outright. This sits ahead of the
	// idempotency check below so a legacy self-peak row cannot be re-confirmed
	// either; feed counts and likedByMe stay untouched, only this write is
	// blocked.
	if activity.UserID == viewer.ID {
		return apperr.Forbidden("you cannot peak your own activity")
	}
	// A repeated like is a no-op, notification included: only the first call
	// owns the row and therefore the notification.
	if _, err := h.Q.GetLike(ctx, activityID, viewer.ID); err == nil {
		count, err := h.Q.CountLikes(ctx, activityID)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, api.LikeResult{LikeCount: count, LikedByMe: true})
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err := h.Q.InsertLike(ctx, activityID, viewer.ID, time.Now().Unix()); err != nil {
		return err
	}
	if err := h.notify(ctx, activity.UserID, viewer.ID, KindLike, &activityID, nil); err != nil {
		return err
	}
	count, err := h.Q.CountLikes(ctx, activityID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, api.LikeResult{LikeCount: count, LikedByMe: true})
}

// visibleActivity loads an activity the viewer is allowed to see. An invisible
// activity is reported as missing rather than forbidden, so the endpoint cannot
// be used to probe for private ids.
func (h *Handlers) visibleActivity(ctx context.Context, activityID, viewerID int64) (db.Activity, error) {
	activity, err := h.Q.GetActivity(ctx, activityID)
	if errors.Is(err, sql.ErrNoRows) {
		return activity, apperr.NotFound("no such activity")
	}
	if err != nil {
		return activity, err
	}
	owner, err := h.Q.GetUserByID(ctx, activity.UserID)
	if err != nil {
		return activity, err
	}
	follower := false
	if viewerID != 0 && viewerID != owner.ID {
		follow, err := h.Q.GetFollow(ctx, viewerID, owner.ID)
		switch {
		case err == nil:
			follower = follow.Status == "accepted"
		case errors.Is(err, sql.ErrNoRows):
		default:
			return activity, err
		}
	}
	if !VisibilityAllows(viewerID, owner.ID, follower, activity.Visibility, owner.ActivitiesVisibility) {
		return activity, apperr.NotFound("no such activity")
	}
	return activity, nil
}
