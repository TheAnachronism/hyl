// Package dto converts database rows into the API types defined in
// internal/api, and reads the shared query parameters.
package dto

import (
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/markbeep/hyl/internal/api"
	"github.com/markbeep/hyl/internal/apperr"
	"github.com/markbeep/hyl/internal/db"
)

// Pagination defaults. Comment, notification and follower lists are
// keyset-paginated by id; activity lists are page-numbered by started_at.
const (
	DefaultLimit = 20
	MaxLimit     = 100
)

// SearchMaxLen caps the ?q= filter, so a pathological value cannot turn the
// LIKE scan into a full-table slog.
const SearchMaxLen = 100

// Search reads ?q=, trimmed of surrounding whitespace and capped at
// SearchMaxLen characters; an empty result disables the text filter.
func Search(c echo.Context) string {
	value := strings.TrimSpace(c.QueryParam("q"))
	if runes := []rune(value); len(runes) > SearchMaxLen {
		value = string(runes[:SearchMaxLen])
	}
	return value
}

// MediaURL builds the URL of one image variant.
func MediaURL(mediaID int64, variant string) string {
	if mediaID <= 0 {
		return ""
	}
	return "/api/media/" + strconv.FormatInt(mediaID, 10) + "?variant=" + variant
}

// AvatarURL is the thumbnail variant; avatars are never rendered larger than
// the 400 px thumbnail.
func AvatarURL(mediaID int64) string { return MediaURL(mediaID, "thumb") }

// PhotoFromMedia renders one photo row.
func PhotoFromMedia(m db.Medium) api.Photo {
	return api.Photo{
		ID:       m.ID,
		URL:      MediaURL(m.ID, "full"),
		ThumbURL: MediaURL(m.ID, "thumb"),
		Width:    int32(m.Width),
		Height:   int32(m.Height),
	}
}

// PhotosFromMedia renders a media list, keeping at most limit entries when
// limit > 0.
func PhotosFromMedia(rows []db.Medium, limit int) []api.Photo {
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]api.Photo, 0, len(rows))
	for _, row := range rows {
		out = append(out, PhotoFromMedia(row))
	}
	return out
}

// Me renders a user's own profile.
func Me(u db.User, avatarMediaID int64) api.Me {
	return api.Me{
		ID:                   u.ID,
		Username:             u.Username,
		Email:                u.Email,
		DisplayName:          u.DisplayName,
		Bio:                  u.Bio,
		AvatarURL:            AvatarURL(avatarMediaID),
		EmailVerified:        u.EmailVerified,
		IsAdmin:              u.IsAdmin,
		ProfileVisibility:    u.ProfileVisibility,
		ActivitiesVisibility: u.ActivitiesVisibility,
		FollowPolicy:         u.FollowPolicy,
		MentionPolicy:        u.MentionPolicy,
		TrimScope:            u.TrimScope,
		TrimRadiusM:          u.TrimRadiusM,
		CreatedAt:            api.Timestamp(u.CreatedAt),
	}
}

// UserRef renders the compact reference used in lists and notifications.
func UserRef(u db.User, avatarMediaID int64) api.UserRef {
	return api.UserRef{
		Username:    u.Username,
		DisplayName: u.DisplayName,
		AvatarURL:   AvatarURL(avatarMediaID),
	}
}

// FollowState renders the viewer's relationship to another user.
func FollowState(viewerID, targetID int64, isFollowing, incomingPending, outgoingPending bool) string {
	switch {
	case viewerID != 0 && viewerID == targetID:
		return "self"
	case isFollowing:
		return "following"
	case outgoingPending:
		return "pending_outgoing"
	case incomingPending:
		return "pending_incoming"
	default:
		return "none"
	}
}

// Limit reads ?limit= with the shared default and cap.
func Limit(c echo.Context) int64 {
	raw := c.QueryParam("limit")
	if raw == "" {
		return DefaultLimit
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return DefaultLimit
	}
	if n > MaxLimit {
		return MaxLimit
	}
	return n
}

// Page reads ?page= (1-based) and clamps it against the number of pages implied
// by total and the effective pageSize, so an out-of-range link still resolves.
// A malformed or absent value is page 1. It returns the effective page and the
// page count; with no rows totalPages is 0 and the page is 1.
func Page(c echo.Context, total, pageSize int64) (page, totalPages int64) {
	totalPages = 0
	if total > 0 && pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	page = 1
	if raw := c.QueryParam("page"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			page = n
		}
	}
	if page < 1 {
		page = 1
	}
	if totalPages > 0 && page > totalPages {
		page = totalPages
	}
	return page, totalPages
}

// Before reads ?before= for the keyset lists (comments, notifications),
// returning 0 when absent or malformed.
func Before(c echo.Context) int64 {
	n, err := strconv.ParseInt(c.QueryParam("before"), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// IDParam reads a positive int64 path parameter.
func IDParam(c echo.Context, name string) (int64, error) {
	n, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || n <= 0 {
		return 0, apperr.BadRequest("invalid %s", name)
	}
	return n, nil
}

// IntQuery reads an optional integer query parameter; 0 means "absent".
func IntQuery(c echo.Context, name string) int64 {
	n, err := strconv.ParseInt(c.QueryParam(name), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// NextBefore returns the keyset cursor for a comment or notification page that
// may have more rows.
func NextBefore(lastID int64, count, limit int64) *int64 {
	if count < limit || count == 0 {
		return nil
	}
	return &lastID
}

// Ptr returns a pointer to v, for optional API fields.
func Ptr[T any](v T) *T { return &v }
