package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"

	"github.com/markbeep/hyl/internal/apperr"
	"github.com/markbeep/hyl/internal/db"
)

// Cookie names used by the OAuth handshake. Both are short-lived and only ever
// carry a provider name and a relative path.
const (
	linkIntentCookie = "hyl_link_intent"
	nextPathCookie   = "hyl_next"
)

// BeginAuth starts an OAuth round trip. It also remembers an optional ?next=
// path so the user lands where they started after the callback.
func (s *Service) BeginAuth(c echo.Context) error {
	provider := c.Param("provider")
	if !s.ProviderEnabled(provider) {
		return apperr.NotFound("unknown provider %q", provider)
	}
	if next := safeNext(c.QueryParam("next")); next != "" {
		c.SetCookie(&http.Cookie{
			Name: nextPathCookie, Value: next, Path: "/", MaxAge: int(handshakeTTL.Seconds()),
			HttpOnly: true, Secure: s.Cfg.IsHTTPS(), SameSite: http.SameSiteLaxMode,
		})
	}

	// echo populates none of the sources gothic.GetProviderName inspects, so the
	// provider must be injected into the request context first.
	req := gothic.GetContextWithProvider(c.Request(), provider)
	gothic.BeginAuthHandler(c.Response(), req)
	return nil
}

// StartLink marks the next OAuth round trip as an identity link for the current
// session instead of a sign-in.
func (s *Service) StartLink(c echo.Context) error {
	provider := c.Param("provider")
	if !s.ProviderEnabled(provider) {
		return apperr.NotFound("unknown provider %q", provider)
	}
	if _, err := s.requireSession(c); err != nil {
		return err
	}
	c.SetCookie(&http.Cookie{
		Name: linkIntentCookie, Value: provider, Path: "/", MaxAge: int(handshakeTTL.Seconds()),
		HttpOnly: true, Secure: s.Cfg.IsHTTPS(), SameSite: http.SameSiteLaxMode,
	})
	return c.Redirect(http.StatusTemporaryRedirect, "/auth/"+provider)
}

// CompleteAuth finishes an OAuth round trip: link, sign in, or create.
func (s *Service) CompleteAuth(c echo.Context) error {
	provider := c.Param("provider")
	if !s.ProviderEnabled(provider) {
		return apperr.NotFound("unknown provider %q", provider)
	}

	req := gothic.GetContextWithProvider(c.Request(), provider)
	gothUser, err := gothic.CompleteUserAuth(c.Response(), req)
	if err != nil {
		s.Log.Warn("oauth callback failed", zapString("provider", provider), zapErr(err))
		return apperr.BadRequest("the %s sign-in did not complete", provider)
	}
	if gothUser.UserID == "" {
		return apperr.BadRequest("the %s sign-in did not return an account id", provider)
	}

	ctx := c.Request().Context()
	now := time.Now().Unix()
	email := strings.ToLower(strings.TrimSpace(gothUser.Email))

	// (a) explicit link from the settings page
	if cookie, cookieErr := c.Cookie(linkIntentCookie); cookieErr == nil && cookie.Value == provider {
		s.clearCookie(c, linkIntentCookie)
		return s.linkIdentity(c, provider, gothUser, email, now)
	}

	// (b) an identity we already know
	identity, err := s.Q.GetIdentity(ctx, provider, gothUser.UserID)
	switch {
	case err == nil:
		user, err := s.Q.GetUserByID(ctx, identity.UserID)
		if errors.Is(err, sql.ErrNoRows) {
			return apperr.BadRequest("that linked account no longer exists")
		}
		if err != nil {
			return err
		}
		return s.finishLogin(c, user)
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}

	// (c) an existing local account with the same, provider-verified address
	if email != "" && providerAssertsVerifiedEmail(provider, gothUser) {
		existing, err := s.Q.GetUserByEmail(ctx, email)
		switch {
		case err == nil:
			if _, err := s.Q.CreateIdentity(ctx, existing.ID, provider, gothUser.UserID, optionalString(email), now); err != nil && !isUniqueViolation(err) {
				return err
			}
			return s.finishLogin(c, existing)
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}
	}

	// (d) a brand new account
	if email == "" {
		return apperr.BadRequest("%s did not share an email address; add one there and try again", provider)
	}
	if taken, err := s.Q.EmailTaken(ctx, email); err != nil {
		return err
	} else if taken > 0 {
		return apperr.Conflict("an account with %s already exists; sign in with it and link %s from settings", email, provider)
	}
	username, err := s.uniqueUsername(ctx, usernameBase(gothUser, email))
	if err != nil {
		return err
	}
	user, err := s.Q.CreateUser(ctx, db.CreateUserParams{
		Username:      username,
		Email:         email,
		DisplayName:   displayNameFor(gothUser, username),
		PasswordHash:  nil,
		EmailVerified: true, // the provider asserted it
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		return conflictOrInternal(err, "that username or email address is already registered")
	}
	if _, err := s.Q.CreateIdentity(ctx, user.ID, provider, gothUser.UserID, optionalString(email), now); err != nil {
		return err
	}
	s.Log.Info("created a user from an oauth sign-in", zapUser(user.ID), zapString("provider", provider))
	return s.finishLogin(c, user)
}

// linkIdentity attaches a provider account to the signed-in user.
func (s *Service) linkIdentity(c echo.Context, provider string, gothUser goth.User, email string, now int64) error {
	user, err := s.requireSession(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()

	existing, err := s.Q.GetIdentity(ctx, provider, gothUser.UserID)
	switch {
	case err == nil && existing.UserID == user.ID:
		return c.Redirect(http.StatusSeeOther, "/settings")
	case err == nil:
		return apperr.Conflict("that %s account is already linked to another hyl user", provider)
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}

	if _, err := s.Q.CreateIdentity(ctx, user.ID, provider, gothUser.UserID, optionalString(email), now); err != nil {
		if isUniqueViolation(err) {
			return apperr.Conflict("that %s account is already linked to another hyl user", provider)
		}
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/settings")
}

// finishLogin issues an application session and redirects to the SPA.
func (s *Service) finishLogin(c echo.Context, user db.User) error {
	token, err := s.CreateSession(c.Request().Context(), user.ID, c.Request().UserAgent(), c.RealIP())
	if err != nil {
		return err
	}
	s.SetSessionCookie(c, token)

	target := safeNext(s.readCookie(c, nextPathCookie))
	s.clearCookie(c, nextPathCookie)
	if target == "" {
		target = "/"
	}
	return c.Redirect(http.StatusSeeOther, target)
}

// requireSession resolves the session of the current request.
func (s *Service) requireSession(c echo.Context) (*db.User, error) {
	user, err := s.ResolveSession(c.Request().Context(), SessionToken(c))
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, apperr.ErrUnauthorized
	}
	return user, nil
}

func (s *Service) readCookie(c echo.Context, name string) string {
	cookie, err := c.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (s *Service) clearCookie(c echo.Context, name string) {
	c.SetCookie(&http.Cookie{
		Name: name, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.Cfg.IsHTTPS(), SameSite: http.SameSiteLaxMode,
	})
}

// providerAssertsVerifiedEmail reports whether the provider vouches for the
// address it returned.
//
// Google returns an explicit `email_verified` claim. GitHub's profile endpoint
// returns a public address that may be unverified, so it only counts when the
// profile carried no address at all and goth fell back to /user/emails, which
// filters for the primary *verified* address.
func providerAssertsVerifiedEmail(provider string, u goth.User) bool {
	switch provider {
	case "google":
		verified, _ := u.RawData["email_verified"].(bool)
		return verified
	case "github":
		if raw, ok := u.RawData["email"].(string); ok && raw != "" {
			return false
		}
		return u.Email != ""
	default:
		return false
	}
}

// usernameBase derives a candidate username from the provider profile.
func usernameBase(u goth.User, email string) string {
	for _, candidate := range []string{u.NickName, u.Name, strings.Split(email, "@")[0]} {
		if s := sanitizeUsername(candidate); len(s) >= 3 {
			return s
		}
	}
	return "athlete"
}

func displayNameFor(u goth.User, fallback string) string {
	for _, candidate := range []string{u.Name, u.NickName} {
		if strings.TrimSpace(candidate) != "" {
			return truncate(strings.TrimSpace(candidate), 60)
		}
	}
	return fallback
}

// sanitizeUsername lowercases and reduces a string to the username alphabet.
func sanitizeUsername(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.' || r == ' ':
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if len(out) > 30 {
		out = out[:30]
	}
	return out
}

// uniqueUsername appends an underscore suffix until the name is free.
func (s *Service) uniqueUsername(ctx context.Context, base string) (string, error) {
	candidate := base
	for i := 1; i <= 50; i++ {
		taken, err := s.Q.UsernameTaken(ctx, candidate)
		if err != nil {
			return "", err
		}
		if taken == 0 {
			return candidate, nil
		}
		suffix := "_" + strconv.Itoa(i+1)
		prefix := base
		if len(prefix) > 30-len(suffix) {
			prefix = prefix[:30-len(suffix)]
		}
		candidate = prefix + suffix
	}
	token, _, err := randomToken()
	if err != nil {
		return "", err
	}
	return "athlete_" + token[:6], nil
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
