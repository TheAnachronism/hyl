// Package users serves profiles, settings, statistics and the mention
// autocomplete.
package users

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/markbeep/hyl/internal/api"
	"github.com/markbeep/hyl/internal/apperr"
	"github.com/markbeep/hyl/internal/db"
	"github.com/markbeep/hyl/internal/dto"
	"github.com/markbeep/hyl/internal/reqctx"
)

// Field limits, mirroring the frontend form hints.
const (
	maxDisplayNameLen = 60
	maxBioLen         = 500
	maxTrimRadiusM    = 5000
)

var (
	profileVisibilities  = []string{"everyone", "followers"}
	activityVisibilities = []string{"everyone", "followers", "only_me"}
	followPolicies       = []string{"everyone", "on_request"}
	mentionPolicies      = []string{"everyone", "followers", "nobody"}
	trimScopes           = []string{"all", "zones"}
)

// Handlers implements the profile HTTP surface.
type Handlers struct {
	Q   *db.Queries
	Log *zap.Logger
}

// New builds the user handlers.
func New(pool *sql.DB, log *zap.Logger) *Handlers {
	return &Handlers{Q: db.New(pool), Log: log}
}

type updateMeRequest struct {
	DisplayName          *string `json:"displayName"`
	Bio                  *string `json:"bio"`
	ProfileVisibility    *string `json:"profileVisibility"`
	ActivitiesVisibility *string `json:"activitiesVisibility"`
	FollowPolicy         *string `json:"followPolicy"`
	MentionPolicy        *string `json:"mentionPolicy"`
	TrimScope            *string `json:"trimScope"`
	TrimRadiusM          *int64  `json:"trimRadiusM"`
}

// Me returns the authenticated user's own profile.
func (h *Handlers) Me(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	avatarID, err := h.avatarID(c.Request().Context(), user.ID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, dto.Me(*user, avatarID))
}

// UpdateMe patches the profile and privacy settings.
func (h *Handlers) UpdateMe(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	var req updateMeRequest
	if err := c.Bind(&req); err != nil {
		return apperr.ErrInvalidRequest
	}
	ctx := c.Request().Context()

	displayName := user.DisplayName
	if req.DisplayName != nil {
		displayName = strings.TrimSpace(*req.DisplayName)
		if len(displayName) > maxDisplayNameLen {
			return apperr.BadRequest("display name must be at most %d characters", maxDisplayNameLen)
		}
	}
	bio := user.Bio
	if req.Bio != nil {
		bio = strings.TrimSpace(*req.Bio)
		if len(bio) > maxBioLen {
			return apperr.BadRequest("bio must be at most %d characters", maxBioLen)
		}
	}

	profileVisibility, err := pick(req.ProfileVisibility, user.ProfileVisibility, profileVisibilities, "profileVisibility")
	if err != nil {
		return err
	}
	activitiesVisibility, err := pick(req.ActivitiesVisibility, user.ActivitiesVisibility, activityVisibilities, "activitiesVisibility")
	if err != nil {
		return err
	}
	followPolicy, err := pick(req.FollowPolicy, user.FollowPolicy, followPolicies, "followPolicy")
	if err != nil {
		return err
	}
	mentionPolicy, err := pick(req.MentionPolicy, user.MentionPolicy, mentionPolicies, "mentionPolicy")
	if err != nil {
		return err
	}
	trimScope, err := pick(req.TrimScope, user.TrimScope, trimScopes, "trimScope")
	if err != nil {
		return err
	}
	trimRadius := user.TrimRadiusM
	if req.TrimRadiusM != nil {
		trimRadius = *req.TrimRadiusM
		if trimRadius < 0 || trimRadius > maxTrimRadiusM {
			return apperr.BadRequest("trimRadiusM must be between 0 and %d", maxTrimRadiusM)
		}
	}

	now := time.Now().Unix()
	if _, err := h.Q.UpdateUserProfile(ctx, displayName, bio, now, user.ID); err != nil {
		return err
	}
	updated, err := h.Q.UpdateUserPrivacy(ctx, db.UpdateUserPrivacyParams{
		ProfileVisibility:    profileVisibility,
		ActivitiesVisibility: activitiesVisibility,
		FollowPolicy:         followPolicy,
		MentionPolicy:        mentionPolicy,
		TrimScope:            trimScope,
		TrimRadiusM:          trimRadius,
		UpdatedAt:            now,
		ID:                   user.ID,
	})
	if err != nil {
		return err
	}
	avatarID, err := h.avatarID(ctx, updated.ID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, dto.Me(updated, avatarID))
}

// Profile returns another user's public profile, honouring profile_visibility.
func (h *Handlers) Profile(c echo.Context) error {
	ctx := c.Request().Context()
	target, err := h.byUsername(ctx, c.Param("username"))
	if err != nil {
		return err
	}
	viewerID := reqctx.UserID(c)

	isFollowing := false
	outgoingPending := false
	incomingPending := false
	if viewerID != 0 && viewerID != target.ID {
		isFollowing, outgoingPending, incomingPending, err = h.followStates(ctx, viewerID, target.ID)
		if err != nil {
			return err
		}
	}

	profile := api.UserProfile{
		ID:          target.ID,
		Username:    target.Username,
		DisplayName: target.DisplayName,
		FollowState: dto.FollowState(viewerID, target.ID, isFollowing, incomingPending, outgoingPending),
		IsMe:        viewerID != 0 && viewerID == target.ID,
	}

	visible := viewerID == target.ID || target.ProfileVisibility == "everyone" || isFollowing
	if !visible {
		// A private profile leaks nothing beyond the name and avatar.
		avatarID, err := h.avatarID(ctx, target.ID)
		if err != nil {
			return err
		}
		profile.AvatarURL = dto.AvatarURL(avatarID)
		profile.IsPrivate = true
		return c.JSON(http.StatusOK, profile)
	}

	avatarID, err := h.avatarID(ctx, target.ID)
	if err != nil {
		return err
	}
	profile.AvatarURL = dto.AvatarURL(avatarID)
	profile.Bio = target.Bio
	if profile.FollowerCount, err = h.Q.CountFollowers(ctx, target.ID); err != nil {
		return err
	}
	if profile.FollowingCount, err = h.Q.CountFollowing(ctx, target.ID); err != nil {
		return err
	}
	if profile.ActivityCount, err = h.Q.CountUserActivities(ctx, target.ID); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, profile)
}

// Search backs the mention autocomplete.
func (h *Handlers) Search(c echo.Context) error {
	query := strings.TrimSpace(c.QueryParam("q"))
	if query == "" {
		return c.JSON(http.StatusOK, []api.UserRef{})
	}
	rows, err := h.Q.SearchUsers(c.Request().Context(), query, 8)
	if err != nil {
		return err
	}
	items := make([]api.UserRef, 0, len(rows))
	for _, row := range rows {
		avatarID, err := h.avatarID(c.Request().Context(), row.ID)
		if err != nil {
			return err
		}
		items = append(items, dto.UserRef(row, avatarID))
	}
	return c.JSON(http.StatusOK, items)
}

// Stats returns per-sport totals for a user, honouring activities_visibility.
func (h *Handlers) Stats(c echo.Context) error {
	ctx := c.Request().Context()
	target, err := h.byUsername(ctx, c.Param("username"))
	if err != nil {
		return err
	}
	viewerID := reqctx.UserID(c)

	from, err := parseDay(c.QueryParam("from"), false)
	if err != nil {
		return err
	}
	to, err := parseDay(c.QueryParam("to"), true)
	if err != nil {
		return err
	}
	sport := c.QueryParam("sport")

	rows, err := h.Q.GetActivityTotalsBySport(ctx, target.ID, from, to, viewerID)
	if err != nil {
		return err
	}

	response := api.StatsResponse{Sports: []api.SportTotals{}, All: api.SportTotals{Sport: "all"}}
	for _, row := range rows {
		if sport != "" && row.Sport != sport {
			continue
		}
		response.Sports = append(response.Sports, api.SportTotals{
			Sport:          row.Sport,
			Count:          row.ActivityCount,
			DistanceM:      row.DistanceM,
			ElevationGainM: row.ElevationGainM,
			MovingTimeS:    row.MovingTimeS,
			ElapsedTimeS:   row.ElapsedTimeS,
		})
		total := &response.All
		total.Count += row.ActivityCount
		total.DistanceM += row.DistanceM
		total.ElevationGainM += row.ElevationGainM
		total.MovingTimeS += row.MovingTimeS
		total.ElapsedTimeS += row.ElapsedTimeS
	}
	return c.JSON(http.StatusOK, response)
}

func nowUnix() int64 { return time.Now().Unix() }

func (h *Handlers) avatarID(ctx context.Context, userID int64) (int64, error) {
	avatar, err := h.Q.GetActiveAvatar(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return avatar.ID, nil
}

func (h *Handlers) byUsername(ctx context.Context, username string) (db.User, error) {
	user, err := h.Q.GetUserByUsername(ctx, username)
	if errors.Is(err, sql.ErrNoRows) {
		return user, apperr.NotFound("unknown user")
	}
	if err != nil {
		return user, err
	}
	return user, nil
}

func (h *Handlers) followStates(ctx context.Context, viewerID, targetID int64) (isFollowing, outgoingPending, incomingPending bool, err error) {
	outgoing, err := h.Q.GetFollow(ctx, viewerID, targetID)
	switch {
	case err == nil:
		isFollowing = outgoing.Status == "accepted"
		outgoingPending = outgoing.Status == "pending"
	case errors.Is(err, sql.ErrNoRows):
	default:
		return false, false, false, err
	}
	incoming, err := h.Q.GetFollow(ctx, targetID, viewerID)
	switch {
	case err == nil:
		incomingPending = incoming.Status == "pending"
	case errors.Is(err, sql.ErrNoRows):
	default:
		return false, false, false, err
	}
	return isFollowing, outgoingPending, incomingPending, nil
}

// pick validates an optional enum value against its allowed set.
func pick(value *string, current string, allowed []string, field string) (string, error) {
	if value == nil {
		return current, nil
	}
	for _, candidate := range allowed {
		if *value == candidate {
			return *value, nil
		}
	}
	return "", apperr.BadRequest("%s must be one of %s", field, strings.Join(allowed, ", "))
}

// parseDay reads a YYYY-MM-DD day boundary. `to` days are exclusive, so the
// returned timestamp is the start of the following day.
func parseDay(raw string, exclusive bool) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	day, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return 0, apperr.BadRequest("dates must be formatted as YYYY-MM-DD")
	}
	if exclusive {
		day = day.AddDate(0, 0, 1)
	}
	return day.Unix(), nil
}
