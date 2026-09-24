// Package server wires the echo instance: middleware, route table and the
// embedded frontend.
package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"

	"github.com/markbeep/hyl/internal/activity"
	"github.com/markbeep/hyl/internal/api"
	"github.com/markbeep/hyl/internal/apperr"
	"github.com/markbeep/hyl/internal/auth"
	"github.com/markbeep/hyl/internal/config"
	"github.com/markbeep/hyl/internal/developer"
	"github.com/markbeep/hyl/internal/media"
	"github.com/markbeep/hyl/internal/social"
	syncpkg "github.com/markbeep/hyl/internal/sync"
	"github.com/markbeep/hyl/internal/users"
	"github.com/markbeep/hyl/internal/webhooks"
)

// Deps are the collaborators the server needs. Feature handler groups are
// attached as phases land; nil groups are simply not registered.
type Deps struct {
	Cfg          config.Config
	Log          *zap.Logger
	Version      string
	DB           *sql.DB
	Auth         *auth.Service
	Users        *users.Handlers
	Activity     *activity.Handlers
	Social       *social.Handlers
	Media        *media.Handlers
	Developer    *developer.Handlers
	Sync         *syncpkg.Handlers
	Webhooks     *webhooks.Intervals
	StravaEvents *webhooks.Strava
	// Static serves the embedded frontend; it is registered as the catch-all
	// route after every API route.
	Static echo.HandlerFunc
}

// New builds the echo instance.
func New(d Deps) (*echo.Echo, error) {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = ErrorHandler(d.Log)
	e.Validator = validator{}

	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(requestLogger(d.Log))
	e.Use(middleware.CSRFWithConfig(middleware.CSRFConfig{
		TokenLookup:    "header:X-CSRF-Token",
		CookieName:     "_csrf",
		CookiePath:     "/",
		CookieHTTPOnly: false,
		CookieSameSite: http.SameSiteLaxMode,
		CookieMaxAge:   86400,
		CookieSecure:   d.Cfg.IsHTTPS(),
		ContextKey:     "csrf",
		// Machine-to-machine callers hold no cookie and cannot fetch a token:
		// the developer API authenticates with a bearer key and the provider
		// webhooks with their own shared secrets, so cross-site request forgery
		// does not apply to either.
		Skipper: func(c echo.Context) bool {
			path := c.Request().URL.Path
			return strings.HasPrefix(path, "/webhooks/") || strings.HasPrefix(path, "/api/v1/")
		},
	}))
	if d.Auth != nil {
		// Every request knows its viewer, so public pages render the
		// viewer-relative state without a second round trip.
		e.Use(OptionalAuth(d.Auth))
	}

	// Public, unauthenticated endpoints.
	e.GET("/api/health", healthHandler(d))
	e.GET("/api/config", configHandler(d))
	e.GET("/api/csrf", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"token": csrfToken(c)})
	})

	if d.Auth != nil {
		registerAuthRoutes(e, d.Auth)
	}
	registerUserRoutes(e, d)
	registerActivityRoutes(e, d)
	registerSyncRoutes(e, d)
	registerDeveloperRoutes(e, d)

	// Tiles are served for GET and HEAD; pmtiles only ever issues GET, but a
	// HEAD probe is how an operator checks the file is wired up.
	tiles := tilesHandler(d.Cfg.TilesDir)
	e.GET("/tiles/:name", tiles)
	e.HEAD("/tiles/:name", tiles)

	// The embedded frontend is the catch-all and must be registered last so it
	// can never shadow an API route.
	if d.Static != nil {
		e.GET("/*", d.Static)
	}

	return e, nil
}

func registerAuthRoutes(e *echo.Echo, a *auth.Service) {
	e.POST("/api/auth/register", a.Register, rateLimiter())
	e.POST("/api/auth/login", a.Login, rateLimiter())
	e.POST("/api/auth/logout", a.Logout, RequireAuth)
	e.POST("/api/auth/verify", a.VerifyEmail)
	e.POST("/api/auth/verify/resend", a.ResendVerification, RequireAuth, rateLimiter())
	e.POST("/api/auth/password/reset-request", a.RequestPasswordReset, rateLimiter())
	e.POST("/api/auth/password/reset", a.ResetPassword)

	e.GET("/auth/:provider", a.BeginAuth)
	e.GET("/auth/:provider/callback", a.CompleteAuth)

	e.POST("/api/me/email", a.ChangeEmail, RequireAuth)
	e.POST("/api/me/password", a.ChangePassword, RequireAuth)
	e.GET("/api/me/identities", a.ListIdentities, RequireAuth)
	e.POST("/api/me/identities/:provider/link", a.StartLink, RequireAuth)
	e.DELETE("/api/me/identities/:provider", a.UnlinkIdentity, RequireAuth)

	e.GET("/api/me/api-keys", a.ListAPIKeys, RequireAuth)
	e.POST("/api/me/api-keys", a.CreateAPIKey, RequireAuth)
	e.DELETE("/api/me/api-keys/:id", a.RevokeAPIKey, RequireAuth)
}

// registerDeveloperRoutes mounts the documented public API. Every route is
// behind a key and its own 60 request/minute budget.
func registerDeveloperRoutes(e *echo.Echo, d Deps) {
	if d.Developer == nil || d.Auth == nil {
		return
	}
	v1 := e.Group("/api/v1", d.Auth.RequireAPIKey, apiKeyRateLimiter())
	v1.GET("/me", d.Developer.Me)
	v1.POST("/activities", d.Developer.Upload)
	v1.GET("/activities", d.Developer.List)
	v1.GET("/activities/:id", d.Developer.Get)
}

func registerUserRoutes(e *echo.Echo, d Deps) {
	if d.Users != nil {
		e.GET("/api/me", d.Users.Me, RequireAuth)
		e.PATCH("/api/me", d.Users.UpdateMe, RequireAuth)
		// Registered before the :username routes; echo also prefers the static
		// segment, so /api/users/search can never fall through to a profile.
		e.GET("/api/users/search", d.Users.Search, RequireAuth)
		e.GET("/api/users/:username", d.Users.Profile)
		e.GET("/api/users/:username/stats", d.Users.Stats)
		e.GET("/api/me/privacy-zones", d.Users.PrivacyZones, RequireAuth)
		e.POST("/api/me/privacy-zones", d.Users.CreatePrivacyZone, RequireAuth)
		e.DELETE("/api/me/privacy-zones/:id", d.Users.DeletePrivacyZone, RequireAuth)
	}
	if d.Activity != nil {
		e.GET("/api/users/:username/activities", d.Activity.UserActivities)
	}
	if d.Social != nil {
		e.GET("/api/notifications", d.Social.ListNotifications, RequireAuth)
		e.GET("/api/notifications/count", d.Social.CountNotifications, RequireAuth)
		e.POST("/api/notifications/read", d.Social.MarkNotificationsRead, RequireAuth)
		e.POST("/api/users/:username/follow", d.Social.Follow, RequireAuth)
		e.DELETE("/api/users/:username/follow", d.Social.Unfollow, RequireAuth)
		e.POST("/api/users/:username/follow/accept", d.Social.AcceptFollow, RequireAuth)
		e.DELETE("/api/users/:username/follow/request", d.Social.RejectFollow, RequireAuth)
		e.GET("/api/me/followers", d.Social.Followers, RequireAuth)
		e.GET("/api/me/following", d.Social.Following, RequireAuth)
		e.GET("/api/me/follow-requests", d.Social.FollowRequests, RequireAuth)
	}
	if d.Media != nil {
		e.POST("/api/me/avatar", d.Media.UploadAvatar, RequireAuth)
		e.DELETE("/api/me/avatar", d.Media.DeleteAvatar, RequireAuth)
		e.GET("/api/media/:id", d.Media.Serve)
	}
	if d.Media != nil && d.Activity != nil {
		e.POST("/api/activities/:id/photos", d.Media.UploadPhotos, RequireAuth)
		e.DELETE("/api/photos/:id", d.Media.DeletePhoto, RequireAuth)
	}
}

func registerActivityRoutes(e *echo.Echo, d Deps) {
	if d.Activity == nil {
		return
	}
	e.GET("/api/activities", d.Activity.List, RequireAuth)
	e.POST("/api/activities", d.Activity.Upload, RequireAuth)
	e.GET("/api/activities/:id", d.Activity.Get)
	e.PATCH("/api/activities/:id", d.Activity.Update, RequireAuth)
	e.DELETE("/api/activities/:id", d.Activity.Delete, RequireAuth)
	if d.Social != nil {
		e.POST("/api/activities/:id/likes", d.Social.Like, RequireAuth)
		e.GET("/api/activities/:id/comments", d.Social.ListComments)
		e.POST("/api/activities/:id/comments", d.Social.PostComment, RequireAuth)
		e.DELETE("/api/comments/:id", d.Social.DeleteComment, RequireAuth)
	}
}

func registerSyncRoutes(e *echo.Echo, d Deps) {
	if d.Sync != nil {
		e.GET("/api/connections", d.Sync.List, RequireAuth)
		e.POST("/api/connections/intervals/apikey", d.Sync.ConnectIntervalsAPIKey, RequireAuth)
		e.GET("/api/connections/intervals/oauth/start", d.Sync.StartIntervalsOAuth, RequireAuth)
		e.GET("/api/connections/intervals/callback", d.Sync.IntervalsCallback, RequireAuth)
		e.PATCH("/api/connections/:kind", d.Sync.UpdateConnection, RequireAuth)
		e.DELETE("/api/connections/:kind", d.Sync.DeleteConnection, RequireAuth)
		e.GET("/api/import-rules", d.Sync.ListImportRules, RequireAuth)
		e.PUT("/api/import-rules", d.Sync.PutImportRules, RequireAuth)
		e.POST("/api/sync/run", d.Sync.RunSync, RequireAuth)
		e.GET("/api/sync/status", d.Sync.Status, RequireAuth)
		e.GET("/api/connections/strava/oauth/start", d.Sync.StartStravaOAuth, RequireAuth)
		e.GET("/api/connections/strava/callback", d.Sync.StravaCallback, RequireAuth)
		e.POST("/api/activities/:id/export", d.Sync.QueueActivityExport, RequireAuth)
		e.GET("/api/activities/:id/export", d.Sync.ExportState, RequireAuth)
	}
	if d.Webhooks != nil {
		e.POST("/webhooks/intervals", d.Webhooks.Handle)
	}
	if d.StravaEvents != nil {
		e.GET("/webhooks/strava", d.StravaEvents.Verify)
		e.POST("/webhooks/strava", d.StravaEvents.Handle)
	}
}

func csrfToken(c echo.Context) string {
	if token, ok := c.Get("csrf").(string); ok {
		return token
	}
	return ""
}

func healthHandler(d Deps) echo.HandlerFunc {
	return func(c echo.Context) error {
		if d.DB != nil {
			ctx, cancel := contextWithTimeout(c, 2*time.Second)
			defer cancel()
			if err := d.DB.PingContext(ctx); err != nil {
				return Errorf("database unavailable")
			}
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}
}

func configHandler(d Deps) echo.HandlerFunc {
	return func(c echo.Context) error {
		var pmtiles *string
		if d.Cfg.PMTilesFile != "" {
			url := "/tiles/" + d.Cfg.PMTilesFile
			pmtiles = &url
		}
		return c.JSON(http.StatusOK, api.ConfigResponse{
			RegistrationOpen: d.Cfg.RegistrationOpen,
			Providers: []api.ProviderInfo{
				{ID: "google", Enabled: d.Cfg.GoogleEnabled()},
				{ID: "github", Enabled: d.Cfg.GithubEnabled()},
			},
			PMTilesURL:       pmtiles,
			TilesAttribution: "Protomaps © OpenStreetMap",
			Version:          d.Version,
			IntervalsOAuth:   d.Cfg.IntervalsOAuthEnabled(),
			Strava:           d.Cfg.StravaEnabled(),
			Webhooks:         d.Cfg.IntervalsWebhookSecret != "" || d.Cfg.StravaWebhookVerifyToken != "",
		})
	}
}

func requestLogger(log *zap.Logger) echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogURI:       true,
		LogMethod:    true,
		LogStatus:    true,
		LogLatency:   true,
		LogRemoteIP:  true,
		LogRequestID: true,
		LogError:     true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			// The error handler runs after this middleware unwinds, so a
			// handler error has not written a status yet: derive it from the
			// error instead of reporting the default 200.
			status := v.Status
			if v.Error != nil {
				var httpErr *echo.HTTPError
				var appErr *apperr.Error
				switch {
				case errors.As(v.Error, &appErr):
					status = appErr.Status
				case errors.As(v.Error, &httpErr):
					status = httpErr.Code
				default:
					status = http.StatusInternalServerError
				}
			}
			fields := []zap.Field{
				zap.String("method", v.Method),
				zap.String("uri", v.URI),
				zap.Int("status", status),
				zap.Duration("latency", v.Latency),
				zap.String("ip", v.RemoteIP),
				zap.String("request_id", v.RequestID),
			}
			switch {
			case v.Error != nil:
				log.Error("request", append(fields, zap.Error(v.Error))...)
			case status >= 500:
				log.Error("request", fields...)
			case status >= 400:
				log.Warn("request", fields...)
			default:
				log.Info("request", fields...)
			}
			return nil
		},
	})
}
