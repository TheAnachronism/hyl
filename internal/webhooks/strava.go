package webhooks

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/markbeep/hyl/internal/config"
	"github.com/markbeep/hyl/internal/db"
	"github.com/markbeep/hyl/internal/secrets"
	syncpkg "github.com/markbeep/hyl/internal/sync"
)

// stravaVerifyCooldown bounds how often one athlete's deauthorization claim may
// cost a token round trip. Strava signs neither the challenge GET nor the event
// POST, so an unauthenticated caller can replay events, and every check rotates
// the stored refresh token.
const stravaVerifyCooldown = 5 * time.Minute

// Strava handles Strava's subscription verification and deauthorization
// webhook.
//
// Strava signs neither the verification GET nor the event POST, and athlete ids
// are public, so the event body is not evidence that anything happened: an
// attacker who knows an athlete id could otherwise delete that athlete's
// connection, its queued exports and its import rules with one request. A claim
// is therefore confirmed with Strava, whose token endpoint is the only
// authority on whether access still exists.
type Strava struct {
	Q      *db.Queries
	Cfg    config.Config
	Log    *zap.Logger
	Cipher *secrets.Cipher

	mu       sync.Mutex
	verified map[int64]time.Time
}

// NewStrava builds the Strava webhook handler.
func NewStrava(pool *sql.DB, cfg config.Config, log *zap.Logger, cipher *secrets.Cipher) *Strava {
	return &Strava{Q: db.New(pool), Cfg: cfg, Log: log, Cipher: cipher}
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
	conn, err := h.Q.GetConnectionByExternalID(ctx, syncpkg.KindStravaOAuth, &athleteID)
	if errors.Is(err, sql.ErrNoRows) {
		h.Log.Debug("deauthorization for an unknown athlete", zap.Int64("owner_id", payload.OwnerID))
		return c.NoContent(http.StatusOK)
	}
	if err != nil {
		return c.NoContent(http.StatusOK)
	}
	if !h.claimDue(payload.OwnerID) {
		h.Log.Debug("ignoring a repeated deauthorization claim", zap.Int64("owner_id", payload.OwnerID))
		return c.NoContent(http.StatusOK)
	}
	if !h.confirmedRevoked(ctx, conn) {
		return c.NoContent(http.StatusOK)
	}

	if _, err := h.Q.DeletePendingExportsForUser(ctx, conn.UserID); err != nil {
		h.Log.Warn("deleting pending exports failed", zap.Error(err))
	}
	if _, err := h.Q.DeleteImportRulesForConnection(ctx, conn.UserID, syncpkg.KindStravaOAuth); err != nil {
		h.Log.Warn("deleting import rules failed", zap.Error(err))
	}
	if _, err := h.Q.DeleteConnection(ctx, conn.UserID, syncpkg.KindStravaOAuth); err != nil {
		h.Log.Error("deleting the Strava connection failed", zap.Error(err))
		return c.NoContent(http.StatusOK)
	}
	h.Log.Info("strava connection removed after deauthorization", zap.Int64("user_id", conn.UserID))
	return c.NoContent(http.StatusOK)
}

// confirmedRevoked asks Strava whether this connection's access was really
// revoked. Only a rejection of the stored refresh token proves it: a transient
// failure proves nothing, and a successful refresh proves the claim was forged.
func (h *Strava) confirmedRevoked(ctx context.Context, conn db.Connection) bool {
	refreshToken, err := h.Cipher.DecryptString(conn.RefreshTokenCipher)
	if err != nil || refreshToken == "" {
		h.Log.Warn("cannot confirm a deauthorization without a stored refresh token",
			zap.Int64("user_id", conn.UserID), zap.Error(err))
		return false
	}

	tokens, err := syncpkg.CheckStravaRefresh(ctx, h.Cfg, refreshToken)
	if err == nil {
		// Strava still honours the credential, so the event was not
		// authoritative. The check rotated the pair, and Strava invalidates the
		// previous refresh token the moment it issues a new one, so the fresh
		// pair has to be stored or the connection would be bricked.
		h.persistRotation(ctx, conn, tokens)
		h.Log.Info("ignored an unconfirmed strava deauthorization", zap.Int64("user_id", conn.UserID))
		return false
	}

	var providerErr *syncpkg.ProviderError
	if !errors.As(err, &providerErr) {
		h.Log.Warn("could not confirm a strava deauthorization", zap.Error(err))
		return false
	}
	switch providerErr.StatusCode {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
		// An unknown or invalid refresh token: the revocation is genuine.
		return true
	}
	h.Log.Warn("could not confirm a strava deauthorization", zap.Error(err))
	return false
}

// persistRotation stores the token pair a confirmation refresh produced.
func (h *Strava) persistRotation(ctx context.Context, conn db.Connection, tokens syncpkg.StravaTokens) {
	access, err := h.Cipher.EncryptString(tokens.AccessToken)
	if err != nil {
		h.Log.Error("encrypting the rotated access token failed", zap.Error(err))
		return
	}
	refresh, err := h.Cipher.EncryptString(tokens.RefreshToken)
	if err != nil {
		h.Log.Error("encrypting the rotated refresh token failed", zap.Error(err))
		return
	}
	expiresAt := tokens.ExpiresAt
	if _, err := h.Q.UpdateConnectionTokens(ctx, access, refresh, &expiresAt, time.Now().Unix(), conn.ID); err != nil {
		h.Log.Error("persisting the rotated strava tokens failed", zap.Error(err))
	}
}

// claimDue reports whether this athlete's claim should be checked now, and
// records the attempt so a replayed event cannot make hyl hammer Strava.
func (h *Strava) claimDue(ownerID int64) bool {
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.verified == nil {
		h.verified = make(map[int64]time.Time)
	}
	if last, ok := h.verified[ownerID]; ok && now.Sub(last) < stravaVerifyCooldown {
		return false
	}
	if len(h.verified) > 512 {
		for id, at := range h.verified {
			if now.Sub(at) >= stravaVerifyCooldown {
				delete(h.verified, id)
			}
		}
	}
	h.verified[ownerID] = now
	return true
}
