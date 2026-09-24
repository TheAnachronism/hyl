// Package config loads hyl's runtime configuration from the environment.
//
// Configuration is environment-only: there is no config file and no dotenv
// loader. Every variable documented in the plan is read exactly once at boot.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config is the fully resolved runtime configuration.
type Config struct {
	// Version is the build version, injected by main from -ldflags.
	Version string

	BaseURL     string
	ListenAddr  string
	DataDir     string
	DBPath      string
	MediaDir    string
	TilesDir    string
	PMTilesFile string
	SecretKey   string
	Env         string
	LogLevel    string

	RegistrationOpen bool
	SessionTTL       time.Duration
	SyncInterval     time.Duration

	GoogleKey    string
	GoogleSecret string
	GithubKey    string
	GithubSecret string

	IntervalsClientID      string
	IntervalsClientSecret  string
	IntervalsWebhookSecret string

	StravaClientID           string
	StravaClientSecret       string
	StravaWebhookVerifyToken string

	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
	SMTPFrom string
	SMTPTLS  string
}

// Load reads the environment and validates the result.
func Load() (Config, error) {
	var c Config

	c.BaseURL = envOr("HYL_BASE_URL", "http://localhost:8080")
	c.ListenAddr = envOr("HYL_LISTEN", ":8080")
	c.DataDir = envOr("HYL_DATA_DIR", "./data")
	c.DBPath = envOr("HYL_DB_PATH", filepath.Join(c.DataDir, "hyl.db"))
	c.MediaDir = envOr("HYL_MEDIA_DIR", filepath.Join(c.DataDir, "media"))
	c.TilesDir = envOr("HYL_TILES_DIR", filepath.Join(c.DataDir, "tiles"))
	c.PMTilesFile = envOr("HYL_PMTILES_FILE", "")
	c.SecretKey = envOr("HYL_SECRET_KEY", "")
	c.Env = envOr("HYL_ENV", "development")
	c.LogLevel = envOr("HYL_LOG_LEVEL", "info")

	registrationOpen, err := boolEnv("HYL_REGISTRATION_OPEN", true)
	if err != nil {
		return c, err
	}
	c.RegistrationOpen = registrationOpen

	c.SessionTTL, err = durationEnv("HYL_SESSION_TTL", 720*time.Hour)
	if err != nil {
		return c, err
	}
	c.SyncInterval, err = durationEnv("HYL_SYNC_INTERVAL", 15*time.Minute)
	if err != nil {
		return c, err
	}

	c.GoogleKey = envOr("HYL_GOOGLE_KEY", "")
	c.GoogleSecret = envOr("HYL_GOOGLE_SECRET", "")
	c.GithubKey = envOr("HYL_GITHUB_KEY", "")
	c.GithubSecret = envOr("HYL_GITHUB_SECRET", "")
	c.IntervalsClientID = envOr("HYL_INTERVALS_CLIENT_ID", "")
	c.IntervalsClientSecret = envOr("HYL_INTERVALS_CLIENT_SECRET", "")
	c.IntervalsWebhookSecret = envOr("HYL_INTERVALS_WEBHOOK_SECRET", "")
	c.StravaClientID = envOr("HYL_STRAVA_CLIENT_ID", "")
	c.StravaClientSecret = envOr("HYL_STRAVA_CLIENT_SECRET", "")
	c.StravaWebhookVerifyToken = envOr("HYL_STRAVA_WEBHOOK_VERIFY_TOKEN", "")

	c.SMTPHost = envOr("HYL_SMTP_HOST", "")
	c.SMTPUser = envOr("HYL_SMTP_USER", "")
	c.SMTPPass = envOr("HYL_SMTP_PASS", "")
	c.SMTPFrom = envOr("HYL_SMTP_FROM", "")
	c.SMTPTLS = envOr("HYL_SMTP_TLS", "starttls")
	c.SMTPPort, err = intEnv("HYL_SMTP_PORT", 587)
	if err != nil {
		return c, err
	}

	if err := c.Validate(); err != nil {
		return c, err
	}
	return c, nil
}

// Validate fails fast on unusable configuration.
func (c Config) Validate() error {
	if len(c.SecretKey) < 32 {
		return fmt.Errorf("HYL_SECRET_KEY must be at least 32 bytes (got %d)", len(c.SecretKey))
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("HYL_BASE_URL must be an absolute URL: %q", c.BaseURL)
	}
	if c.SMTPHost != "" && c.SMTPFrom == "" {
		return fmt.Errorf("HYL_SMTP_FROM is required when HYL_SMTP_HOST is set")
	}
	switch c.SMTPTLS {
	case "starttls", "implicit", "none":
	default:
		return fmt.Errorf("HYL_SMTP_TLS must be starttls, implicit or none: %q", c.SMTPTLS)
	}
	if c.PMTilesFile != "" && (strings.ContainsAny(c.PMTilesFile, `/\`) || !strings.HasSuffix(c.PMTilesFile, ".pmtiles")) {
		return fmt.Errorf("HYL_PMTILES_FILE must be a bare .pmtiles filename: %q", c.PMTilesFile)
	}
	return nil
}

// EnsureDirs creates the data, media and tile directories.
func (c Config) EnsureDirs() error {
	for _, dir := range []string{c.DataDir, c.MediaDir, c.TilesDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// IsProduction reports whether the instance runs in production mode.
func (c Config) IsProduction() bool { return c.Env == "production" }

// IsHTTPS reports whether the public origin is served over TLS. It drives the
// Secure flag on every cookie hyl sets.
func (c Config) IsHTTPS() bool { return strings.HasPrefix(c.BaseURL, "https://") }

// OAuthCallback is the absolute redirect URI handed to an OAuth provider.
func (c Config) OAuthCallback(provider string) string {
	return strings.TrimSuffix(c.BaseURL, "/") + "/auth/" + provider + "/callback"
}

// GoogleEnabled reports whether both Google credentials are configured.
func (c Config) GoogleEnabled() bool { return c.GoogleKey != "" && c.GoogleSecret != "" }

// GithubEnabled reports whether both GitHub credentials are configured.
func (c Config) GithubEnabled() bool { return c.GithubKey != "" && c.GithubSecret != "" }

// IntervalsOAuthEnabled reports whether the intervals OAuth routes are registered.
func (c Config) IntervalsOAuthEnabled() bool {
	return c.IntervalsClientID != "" && c.IntervalsClientSecret != ""
}

// StravaEnabled reports whether the Strava connect routes are registered.
func (c Config) StravaEnabled() bool { return c.StravaClientID != "" && c.StravaClientSecret != "" }

func envOr(name, def string) string {
	if v, ok := os.LookupEnv(name); ok && v != "" {
		return v
	}
	return def
}

func boolEnv(name string, def bool) (bool, error) {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def, fmt.Errorf("%s must be a boolean: %q", name, v)
	}
	return b, nil
}

func intEnv(name string, def int) (int, error) {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def, fmt.Errorf("%s must be an integer: %q", name, v)
	}
	return n, nil
}

func durationEnv(name string, def time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def, fmt.Errorf("%s must be a Go duration: %q", name, v)
	}
	return d, nil
}
