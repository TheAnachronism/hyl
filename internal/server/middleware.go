package server

import (
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"

	"github.com/markbeep/hyl/internal/apperr"
	"github.com/markbeep/hyl/internal/auth"
	"github.com/markbeep/hyl/internal/reqctx"
)

// rateLimitPerMinute is the documented budget for the credential endpoints.
const rateLimitPerMinute = 20

// OptionalAuth resolves the session cookie into the request context when it is
// present. It runs for every request so public pages can still show the
// viewer-relative state.
func OptionalAuth(s *auth.Service) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user, err := s.ResolveRequest(c)
			if err != nil {
				return err
			}
			if user != nil {
				reqctx.SetUser(c, user)
			}
			return next(c)
		}
	}
}

// RequireAuth rejects anonymous requests with a 401.
func RequireAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if reqctx.User(c) == nil {
			return apperr.ErrUnauthorized
		}
		return next(c)
	}
}

// apiKeyRateLimiter gives each developer key its own budget, so one noisy
// integration cannot exhaust another's.
func apiKeyRateLimiter() echo.MiddlewareFunc {
	store := middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
		Rate:      rate.Limit(1.0), // 60 requests per minute, with a full burst
		Burst:     60,
		ExpiresIn: 3 * time.Minute,
	})
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: store,
		IdentifierExtractor: func(c echo.Context) (string, error) {
			if id := auth.APIKeyID(c); id != 0 {
				return "key:" + strconv.FormatInt(id, 10), nil
			}
			return "ip:" + c.RealIP(), nil
		},
		ErrorHandler: func(c echo.Context, err error) error {
			return apperr.ErrRateLimited
		},
	})
}

// rateLimiter builds a per-IP limiter with its own bucket, so one endpoint
// being hammered cannot lock a user out of another.
func rateLimiter() echo.MiddlewareFunc {
	limit := rate.Limit(rateLimitPerMinute / 60.0)
	store := middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
		Rate:      limit,
		Burst:     10,
		ExpiresIn: 3 * time.Minute,
	})
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: store,
		IdentifierExtractor: func(c echo.Context) (string, error) {
			return c.RealIP(), nil
		},
		ErrorHandler: func(c echo.Context, err error) error {
			return apperr.ErrRateLimited
		},
	})
}
