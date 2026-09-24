package social

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/markbeep/hyl/internal/api"
	"github.com/markbeep/hyl/internal/dto"
	"github.com/markbeep/hyl/internal/reqctx"
)

// ListNotifications returns one page of the caller's notifications, newest
// first.
func (h *Handlers) ListNotifications(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	limit := dto.Limit(c)
	rows, err := h.Q.ListNotifications(ctx, user.ID, dto.Before(c), limit)
	if err != nil {
		return err
	}

	items := make([]api.Notification, 0, len(rows))
	for _, row := range rows {
		var readAt *string
		if row.ReadAt != nil {
			readAt = dto.Ptr(api.Timestamp(*row.ReadAt))
		}
		items = append(items, api.Notification{
			ID:   row.ID,
			Kind: row.Kind,
			Actor: api.UserRef{
				Username:    row.ActorUsername,
				DisplayName: row.ActorDisplayName,
				AvatarURL:   dto.AvatarURL(row.ActorAvatarMediaID),
			},
			ActivityID: row.ActivityID,
			CommentID:  row.CommentID,
			CreatedAt:  api.Timestamp(row.CreatedAt),
			ReadAt:     readAt,
		})
	}
	page := api.NotificationPage{Items: items}
	if len(items) > 0 {
		page.NextBefore = dto.NextBefore(items[len(items)-1].ID, int64(len(items)), limit)
	}
	return c.JSON(http.StatusOK, page)
}

// CountNotifications returns the unread badge.
func (h *Handlers) CountNotifications(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	unread, err := h.Q.CountUnreadNotifications(c.Request().Context(), user.ID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, api.NotificationCount{Unread: unread})
}

// MarkNotificationsRead marks the given ids, or everything when the list is
// empty.
func (h *Handlers) MarkNotificationsRead(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.Bind(&req); err != nil {
		return err
	}
	ctx := c.Request().Context()
	now := time.Now().Unix()

	if len(req.IDs) == 0 {
		if _, err := h.Q.MarkAllNotificationsRead(ctx, &now, user.ID); err != nil {
			return err
		}
		return c.NoContent(http.StatusNoContent)
	}
	for _, id := range req.IDs {
		// The user id in the WHERE clause keeps another account's rows out of
		// reach even if a caller guesses ids.
		if _, err := h.Q.MarkNotificationRead(ctx, &now, id, user.ID); err != nil {
			return err
		}
	}
	return c.NoContent(http.StatusNoContent)
}
