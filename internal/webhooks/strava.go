package webhooks

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/markbeep/hyl/internal/config"
	"github.com/markbeep/hyl/internal/db"
	"github.com/markbeep/hyl/internal/sync"
)

// Strava handles Strava's subscription verification and deauthorization
// webhook. Strava signs neither, so the handler only ever acts on an owner id
// that already belongs to a stored connection.
type Strava struct {
	Q   *db.Queries
	Cfg config.Config
	Log *zap.Logger
}

// NewStrava builds the Strava webhook handler.
func NewStrava(pool *sql.DB, cfg config.Config, log *zap.Logger) *Strava {
	return &Strava{Q: db.New(pool), Cfg: cfg, Log: log}
}

// Verify answers Strava's subscription handshake.
func (h *Strava) Verify(c echo.Context) error {
	if h.Cfg.StravaWebhookVerifyToken == "" {
		return echo.NotFoundHandler(c)
	}
	if c.QueryParam("hub.mode") != "subscribe" ||
		c.QueryParam("hub.verify_token") != h.Cfg.StravaWebhookVerifyToken {
		return echo.NewHTTPError(http.StatusForbidden, "invalid webhook verification")
	}
	return c.JSON(http.StatusOK, map[string]string{"hub.challenge": c.QueryParam("hub.challenge")})
}

// Handle processes a deauthorization event.
func (h *Strava) Handle(c echo.Context) error {
	if h.Cfg.StravaWebhookVerifyToken == "" {
		return echo.NotFoundHandler(c)
	}
	var payload struct {
		ObjectType string `json:"object_type"`
		AspectType string `json:"aspect_type"`
		ObjectID   int64  `json:"object_id"`
		OwnerID    int64  `json:"owner_id"`
		Updates    struct {
			Authorized string `json:"authorized"`
		} `json:"updates"`
	}
	if err := c.Bind(&payload); err != nil {
		return c.NoContent(http.StatusOK)
	}
	// Everything hyl does not act on is acknowledged so Strava stops retrying.
	if payload.ObjectType != "athlete" || payload.OwnerID == 0 || payload.Updates.Authorized != "false" {
		return c.NoContent(http.StatusOK)
	}

	ctx := c.Request().Context()
	athleteID := strconv.FormatInt(payload.OwnerID, 10)
	conn, err := h.Q.GetConnectionByExternalID(ctx, sync.KindStravaOAuth, &athleteID)
	if errors.Is(err, sql.ErrNoRows) {
		h.Log.Debug("deauthorization for an unknown athlete", zap.Int64("owner_id", payload.OwnerID))
		return c.NoContent(http.StatusOK)
	}
	if err != nil {
		return c.NoContent(http.StatusOK)
	}

	if _, err := h.Q.DeletePendingExportsForUser(ctx, conn.UserID); err != nil {
		h.Log.Warn("deleting pending exports failed", zap.Error(err))
	}
	if _, err := h.Q.DeleteImportRulesForConnection(ctx, conn.UserID, sync.KindStravaOAuth); err != nil {
		h.Log.Warn("deleting import rules failed", zap.Error(err))
	}
	if _, err := h.Q.DeleteConnection(ctx, conn.UserID, sync.KindStravaOAuth); err != nil {
		h.Log.Error("deleting the Strava connection failed", zap.Error(err))
		return c.NoContent(http.StatusOK)
	}
	h.Log.Info("strava connection removed after deauthorization", zap.Int64("user_id", conn.UserID))
	return c.NoContent(http.StatusOK)
}
