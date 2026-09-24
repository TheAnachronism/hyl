// Package auth implements hyl's own email/password login, the session cookie,
// transactional mail and the Google/GitHub OAuth providers.
package auth

import (
	"database/sql"

	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/google"
	"go.uber.org/zap"

	"github.com/markbeep/hyl/internal/config"
	"github.com/markbeep/hyl/internal/db"
)

// Email token lifetimes.
const (
	verifyTokenTTL = 24 * 3600
	resetTokenTTL  = 3600
)

// Token kinds stored in email_tokens.kind.
const (
	TokenKindVerify = "verify"
	TokenKindReset  = "reset"
)

// Service owns authentication: credentials, sessions, mail and OAuth.
type Service struct {
	Pool *sql.DB
	Q    *db.Queries
	Cfg  config.Config
	Log  *zap.Logger
	Mail *Mailer

	providers []string
	// dummyHash keeps password verification timing flat for unknown accounts.
	dummyHash string
}

// New builds the auth service and registers the OAuth providers that have
// credentials configured.
func New(pool *sql.DB, cfg config.Config, log *zap.Logger) (*Service, error) {
	mailer, err := NewMailer(cfg, log)
	if err != nil {
		return nil, err
	}
	dummy, err := HashPassword("hyl-timing-equalizer")
	if err != nil {
		return nil, err
	}

	s := &Service{
		Pool:      pool,
		Q:         db.New(pool),
		Cfg:       cfg,
		Log:       log,
		Mail:      mailer,
		dummyHash: dummy,
	}
	s.registerProviders()
	return s, nil
}

// Providers lists the OAuth provider ids that are usable.
func (s *Service) Providers() []string {
	return append([]string(nil), s.providers...)
}

func (s *Service) registerProviders() {
	var providers []goth.Provider
	if s.Cfg.GoogleEnabled() {
		providers = append(providers, google.New(s.Cfg.GoogleKey, s.Cfg.GoogleSecret, s.Cfg.OAuthCallback("google")))
	}
	if s.Cfg.GithubEnabled() {
		providers = append(providers, github.New(s.Cfg.GithubKey, s.Cfg.GithubSecret, s.Cfg.OAuthCallback("github"), "user:email"))
	}
	goth.UseProviders(providers...)
	gothic.Store = newHandshakeStore(s.Cfg.IsHTTPS())

	for _, p := range providers {
		s.providers = append(s.providers, p.Name())
	}
}

// ProviderEnabled reports whether a provider is registered.
func (s *Service) ProviderEnabled(name string) bool {
	for _, p := range s.providers {
		if p == name {
			return true
		}
	}
	return false
}
