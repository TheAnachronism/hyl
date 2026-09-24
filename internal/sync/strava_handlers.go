package sync

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/markbeep/hyl/internal/api"
	"github.com/markbeep/hyl/internal/apperr"
	"github.com/markbeep/hyl/internal/db"
	"github.com/markbeep/hyl/internal/dto"
	"github.com/markbeep/hyl/internal/reqctx"
)

// StartStravaOAuth redirects to Strava's consent screen.
func (h *Handlers) StartStravaOAuth(c echo.Context) error {
	if _, err := reqctx.RequireUser(c); err != nil {
		return err
	}
	if !h.Cfg.StravaEnabled() {
		return apperr.NotFound("Strava is not configured on this instance")
	}
	state, err := randomState()
	if err != nil {
		return err
	}
	c.SetCookie(stateCookie(state, h.Cfg.IsHTTPS()))
	return c.Redirect(http.StatusTemporaryRedirect, StravaAuthorizeURL(h.Cfg, state))
}

// StravaCallback exchanges the code and stores the token pair.
func (h *Handlers) StravaCallback(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	if !h.Cfg.StravaEnabled() {
		return apperr.NotFound("Strava is not configured on this instance")
	}
	state, err := c.Cookie(oauthStateCookie)
	if err != nil || state.Value == "" || state.Value != c.QueryParam("state") {
		return apperr.BadRequest("the Strava authorization did not match this session")
	}
	c.SetCookie(clearedStateCookie(h.Cfg.IsHTTPS()))
	if scopeError := c.QueryParam("error"); scopeError != "" {
		return apperr.BadRequest("Strava refused the authorization: %s", scopeError)
	}

	ctx := c.Request().Context()
	tokens, err := ExchangeStravaCode(ctx, h.Cfg, c.QueryParam("code"))
	if err != nil {
		return err
	}
	if !tokens.GrantsActivityWrite() {
		// The athlete can untick activity:write on the consent screen. Without
		// it every upload is rejected, so refusing the connection now beats a
		// connection that looks healthy and silently fails every export.
		return apperr.BadRequest("Strava did not grant the activity:write permission, so hyl cannot upload activities")
	}
	access, err := h.Cipher.EncryptString(tokens.AccessToken)
	if err != nil {
		return err
	}
	refresh, err := h.Cipher.EncryptString(tokens.RefreshToken)
	if err != nil {
		return err
	}
	athleteID := strconv.FormatInt(tokens.Athlete.ID, 10)
	now := time.Now().Unix()
	if _, err := h.Q.UpsertConnection(ctx, db.UpsertConnectionParams{
		UserID: user.ID, Kind: KindStravaOAuth, ExternalAthleteID: &athleteID,
		AccessTokenCipher: access, RefreshTokenCipher: refresh, TokenExpiresAt: &tokens.ExpiresAt,
		AutoExport: false, ExportMessage: "Imported from hyl", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/settings")
}

// QueueActivityExport enqueues one activity for the given target.
func (h *Handlers) QueueActivityExport(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	activityID, err := dto.IDParam(c, "id")
	if err != nil {
		return err
	}
	var req struct {
		Target string `json:"target"`
	}
	if err := c.Bind(&req); err != nil {
		return apperr.ErrInvalidRequest
	}
	if req.Target != exportTargetStrava {
		return apperr.BadRequest("target must be strava")
	}
	if !h.Cfg.StravaEnabled() {
		return apperr.BadRequest("Strava is not configured on this instance")
	}

	ctx := c.Request().Context()
	activityRow, err := h.Q.GetActivity(ctx, activityID)
	if errors.Is(err, sql.ErrNoRows) {
		return apperr.NotFound("no such activity")
	}
	if err != nil {
		return err
	}
	if activityRow.UserID != user.ID {
		return apperr.Forbidden("only the owner can export this activity")
	}
	if _, err := h.Q.GetConnection(ctx, user.ID, KindStravaOAuth); errors.Is(err, sql.ErrNoRows) {
		return apperr.BadRequest("connect Strava before exporting")
	} else if err != nil {
		return err
	}

	queued, err := QueueExport(ctx, h.Q, activityRow, req.Target)
	if err != nil {
		return err
	}
	if !queued {
		return apperr.Conflict("this activity is already queued for Strava")
	}
	h.Worker.Trigger(user.ID)
	return c.JSON(http.StatusAccepted, map[string]string{"status": "queued"})
}

// ExportState reports the export row of one activity, if any.
func (h *Handlers) ExportState(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	activityID, err := dto.IDParam(c, "id")
	if err != nil {
		return err
	}
	row, err := h.Q.GetExport(c.Request().Context(), activityID, exportTargetStrava)
	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(http.StatusOK, api.ExportState{})
	}
	if err != nil {
		return err
	}
	if row.UserID != user.ID {
		return apperr.NotFound("no such export")
	}
	return c.JSON(http.StatusOK, api.ExportState{
		Target:    row.Target,
		Status:    row.Status,
		LastError: row.LastError,
		UpdatedAt: api.Timestamp(row.UpdatedAt),
	})
}

func stateCookie(state string, secure bool) *http.Cookie {
	return &http.Cookie{
		Name: oauthStateCookie, Value: state, Path: "/", MaxAge: 900,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	}
}

func clearedStateCookie(secure bool) *http.Cookie {
	cookie := stateCookie("", secure)
	cookie.MaxAge = -1
	return cookie
}
