package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/markbeep/hyl/internal/api"
	"github.com/markbeep/hyl/internal/apperr"
	"github.com/markbeep/hyl/internal/db"
	"github.com/markbeep/hyl/internal/reqctx"
)

// apiKeyPrefix marks a hyl developer key and makes it identifiable in logs.
const apiKeyPrefix = "hyl_"

// apiKeyTouchInterval throttles the last_used_at write to one per minute.
const apiKeyTouchInterval = 60

// APIKeyContextKey carries the authenticated key id for the rate limiter.
const APIKeyContextKey = "hyl.apikey.id"

// GenerateAPIKey returns the plaintext key (shown once), its hash (the only
// stored form) and the display prefix.
func GenerateAPIKey() (plaintext string, hash []byte, prefix string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(buf)
	plaintext = apiKeyPrefix + encoded
	sum := sha256.Sum256([]byte(plaintext))
	if len(encoded) < 8 {
		return "", nil, "", errors.New("generated key is too short")
	}
	return plaintext, sum[:], encoded[:8], nil
}

// RequireAPIKey authenticates a developer request from its bearer token.
func (s *Service) RequireAPIKey(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		raw := bearerToken(c.Request().Header.Get(echo.HeaderAuthorization))
		if raw == "" {
			return apperr.ErrUnauthorized
		}
		sum := sha256.Sum256([]byte(raw))
		ctx := c.Request().Context()

		key, err := s.Q.GetAPIKeyByHash(ctx, sum[:])
		if errors.Is(err, sql.ErrNoRows) {
			return apperr.ErrUnauthorized
		}
		if err != nil {
			return err
		}
		if key.RevokedAt != nil {
			return apperr.ErrUnauthorized
		}

		user, err := s.Q.GetUserByID(ctx, key.UserID)
		if errors.Is(err, sql.ErrNoRows) {
			return apperr.ErrUnauthorized
		}
		if err != nil {
			return err
		}

		c.Set(APIKeyContextKey, key.ID)
		reqctx.SetUser(c, &user)

		// last_used_at is bookkeeping, not authentication: writing it on every
		// request would turn a read-only API call into a write.
		now := time.Now().Unix()
		if key.LastUsedAt == nil || now-*key.LastUsedAt >= apiKeyTouchInterval {
			if err := s.Q.TouchAPIKey(ctx, &now, key.ID); err != nil {
				s.Log.Warn("updating the API key timestamp failed", zapErr(err))
			}
		}
		return next(c)
	}
}

// APIKeyID is the authenticated key id, or 0 when the request used a session.
func APIKeyID(c echo.Context) int64 {
	id, _ := c.Get(APIKeyContextKey).(int64)
	return id
}

// ListAPIKeys returns the caller's active keys, never the secret.
func (s *Service) ListAPIKeys(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	rows, err := s.Q.ListAPIKeys(c.Request().Context(), user.ID)
	if err != nil {
		return err
	}
	out := make([]api.APIKey, 0, len(rows))
	for _, row := range rows {
		out = append(out, apiKeyDTO(row))
	}
	return c.JSON(http.StatusOK, out)
}

// CreateAPIKey issues a key and returns the plaintext exactly once.
func (s *Service) CreateAPIKey(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return apperr.ErrInvalidRequest
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return apperr.BadRequest("give the key a name")
	}
	if len(name) > 60 {
		return apperr.BadRequest("the key name must be at most 60 characters")
	}

	plaintext, hash, prefix, err := GenerateAPIKey()
	if err != nil {
		return err
	}
	row, err := s.Q.CreateAPIKey(c.Request().Context(), user.ID, name, hash, prefix, time.Now().Unix())
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, api.APIKeyCreated{APIKey: apiKeyDTO(row), Key: plaintext})
}

// RevokeAPIKey retires one of the caller's keys.
func (s *Service) RevokeAPIKey(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	keyID, err := parseID(c.Param("id"))
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	affected, err := s.Q.RevokeAPIKey(c.Request().Context(), &now, keyID, user.ID)
	if err != nil {
		return err
	}
	if affected == 0 {
		return apperr.NotFound("no such API key")
	}
	return c.NoContent(http.StatusNoContent)
}

func apiKeyDTO(row db.ApiKey) api.APIKey {
	return api.APIKey{
		ID:         row.ID,
		Name:       row.Name,
		Prefix:     row.Prefix,
		CreatedAt:  api.Timestamp(row.CreatedAt),
		LastUsedAt: api.TimestampPtr(row.LastUsedAt),
	}
}

func bearerToken(header string) string {
	const scheme = "Bearer "
	if len(header) <= len(scheme) || !strings.EqualFold(header[:len(scheme)], scheme) {
		return ""
	}
	return strings.TrimSpace(header[len(scheme):])
}

func parseID(raw string) (int64, error) {
	var value int64
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, apperr.BadRequest("invalid id")
		}
		value = value*10 + int64(r-'0')
	}
	if value <= 0 {
		return 0, apperr.BadRequest("invalid id")
	}
	return value, nil
}
