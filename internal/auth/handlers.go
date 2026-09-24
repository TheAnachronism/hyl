package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/markbeep/hyl/internal/api"
	"github.com/markbeep/hyl/internal/apperr"
	"github.com/markbeep/hyl/internal/db"
	"github.com/markbeep/hyl/internal/dto"
	"github.com/markbeep/hyl/internal/reqctx"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9_]{3,30}$`)

// errBadCredentials is deliberately identical for unknown users, wrong
// passwords and OAuth-only accounts.
var errBadCredentials = apperr.New(apperr.CodeUnauthorized, http.StatusUnauthorized, "invalid username or password")

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type tokenRequest struct {
	Token string `json:"token"`
}

type resetRequestBody struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type changeEmailRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// Register creates a local account, makes the first user an admin, sends the
// verification mail and signs the new user in.
func (s *Service) Register(c echo.Context) error {
	if !s.Cfg.RegistrationOpen {
		return apperr.Forbidden("registration is closed")
	}
	var req registerRequest
	if err := c.Bind(&req); err != nil {
		return apperr.ErrInvalidRequest
	}

	username := strings.ToLower(strings.TrimSpace(req.Username))
	if !usernamePattern.MatchString(username) {
		return apperr.BadRequest("username must be 3-30 characters of a-z, 0-9 or _")
	}
	email, err := normalizeEmail(req.Email)
	if err != nil {
		return err
	}
	if err := ValidatePasswordStrength(req.Password); err != nil {
		return apperr.BadRequest("%s", err.Error())
	}

	ctx := c.Request().Context()
	if n, err := s.Q.UsernameTaken(ctx, username); err != nil {
		return err
	} else if n > 0 {
		return apperr.Conflict("that username is taken")
	}
	if n, err := s.Q.EmailTaken(ctx, email); err != nil {
		return err
	} else if n > 0 {
		return apperr.Conflict("that email address is already registered")
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	user, err := s.Q.CreateUser(ctx, db.CreateUserParams{
		Username:      username,
		Email:         email,
		DisplayName:   username,
		PasswordHash:  &hash,
		EmailVerified: false,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		return conflictOrInternal(err, "that username or email address is already registered")
	}

	s.sendVerification(ctx, user)

	token, err := s.CreateSession(ctx, user.ID, c.Request().UserAgent(), c.RealIP())
	if err != nil {
		return err
	}
	s.SetSessionCookie(c, token)

	me, err := s.meResponse(ctx, user)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, me)
}

// Login accepts a username or an email address.
func (s *Service) Login(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return apperr.ErrInvalidRequest
	}
	login := strings.TrimSpace(req.Login)
	if login == "" || req.Password == "" {
		return errBadCredentials
	}

	ctx := c.Request().Context()
	user, err := s.lookupUser(ctx, login)
	if err != nil {
		return err
	}

	if user == nil || user.PasswordHash == nil {
		// Keep the timing of an unknown account indistinguishable from a wrong
		// password, and never reveal that an account exists.
		_, _ = VerifyPassword(s.dummyHash, req.Password)
		return errBadCredentials
	}
	ok, err := VerifyPassword(*user.PasswordHash, req.Password)
	if err != nil {
		s.Log.Error("stored password hash is unusable", zapUser(user.ID))
		return errBadCredentials
	}
	if !ok {
		return errBadCredentials
	}

	token, err := s.CreateSession(ctx, user.ID, c.Request().UserAgent(), c.RealIP())
	if err != nil {
		return err
	}
	s.SetSessionCookie(c, token)

	me, err := s.meResponse(ctx, *user)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, me)
}

// Logout deletes the session row and clears the cookie.
func (s *Service) Logout(c echo.Context) error {
	if _, err := reqctx.RequireUser(c); err != nil {
		return err
	}
	if err := s.DeleteSession(c.Request().Context(), SessionToken(c)); err != nil {
		return err
	}
	s.ClearSessionCookie(c)
	return c.NoContent(http.StatusNoContent)
}

// VerifyEmail consumes a verification token and marks the account verified.
func (s *Service) VerifyEmail(c echo.Context) error {
	var req tokenRequest
	if err := c.Bind(&req); err != nil {
		return apperr.ErrInvalidRequest
	}
	ctx := c.Request().Context()
	token, err := s.consumeToken(ctx, req.Token, TokenKindVerify)
	if err != nil {
		return err
	}
	if _, err := s.Q.MarkEmailVerified(ctx, time.Now().Unix(), token.UserID); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// ResendVerification issues a fresh verification mail for the current user.
func (s *Service) ResendVerification(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	if user.EmailVerified {
		return c.NoContent(http.StatusNoContent)
	}
	s.sendVerification(c.Request().Context(), *user)
	return c.NoContent(http.StatusNoContent)
}

// RequestPasswordReset always answers 204 so the endpoint cannot be used to
// enumerate accounts.
func (s *Service) RequestPasswordReset(c echo.Context) error {
	var req resetRequestBody
	if err := c.Bind(&req); err != nil {
		return apperr.ErrInvalidRequest
	}
	ctx := c.Request().Context()

	email, err := normalizeEmail(req.Email)
	if err != nil {
		return c.NoContent(http.StatusNoContent)
	}
	user, err := s.Q.GetUserByEmail(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		s.Log.Info("password reset requested for an unknown address")
		return c.NoContent(http.StatusNoContent)
	}
	if err != nil {
		return err
	}
	s.sendPasswordReset(ctx, user)
	return c.NoContent(http.StatusNoContent)
}

// ResetPassword consumes a reset token and replaces the password, invalidating
// every existing session.
func (s *Service) ResetPassword(c echo.Context) error {
	var req resetPasswordRequest
	if err := c.Bind(&req); err != nil {
		return apperr.ErrInvalidRequest
	}
	if err := ValidatePasswordStrength(req.Password); err != nil {
		return apperr.BadRequest("%s", err.Error())
	}
	ctx := c.Request().Context()

	token, err := s.consumeToken(ctx, req.Token, TokenKindReset)
	if err != nil {
		return err
	}
	hash, err := HashPassword(req.Password)
	if err != nil {
		return err
	}
	if _, err := s.Q.UpdateUserPassword(ctx, &hash, time.Now().Unix(), token.UserID); err != nil {
		return err
	}
	if err := s.DeleteUserSessions(ctx, token.UserID); err != nil {
		return err
	}
	s.ClearSessionCookie(c)
	return c.NoContent(http.StatusNoContent)
}

// ChangeEmail re-verifies the password, sets the new address and sends a fresh
// verification mail.
func (s *Service) ChangeEmail(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	var req changeEmailRequest
	if err := c.Bind(&req); err != nil {
		return apperr.ErrInvalidRequest
	}
	if user.PasswordHash == nil {
		return apperr.BadRequest("set a password before changing your email address")
	}
	if err := s.checkPassword(*user, req.Password); err != nil {
		return err
	}
	email, err := normalizeEmail(req.Email)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	if taken, err := s.Q.EmailTaken(ctx, email); err != nil {
		return err
	} else if taken > 0 {
		return apperr.Conflict("that email address is already registered")
	}

	now := time.Now().Unix()
	if _, err := s.Q.UpdateUserEmail(ctx, email, false, now, user.ID); err != nil {
		return conflictOrInternal(err, "that email address is already registered")
	}
	updated, err := s.Q.GetUserByID(ctx, user.ID)
	if err != nil {
		return err
	}
	s.sendVerification(ctx, updated)

	me, err := s.meResponse(ctx, updated)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, me)
}

// ChangePassword re-verifies the current password and signs other sessions out.
func (s *Service) ChangePassword(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	var req changePasswordRequest
	if err := c.Bind(&req); err != nil {
		return apperr.ErrInvalidRequest
	}
	if user.PasswordHash == nil {
		return apperr.BadRequest("set a password before changing it")
	}
	if err := s.checkPassword(*user, req.CurrentPassword); err != nil {
		return err
	}
	if err := ValidatePasswordStrength(req.NewPassword); err != nil {
		return apperr.BadRequest("%s", err.Error())
	}

	hash, err := HashPassword(req.NewPassword)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	if _, err := s.Q.UpdateUserPassword(ctx, &hash, time.Now().Unix(), user.ID); err != nil {
		return err
	}
	if err := s.DeleteOtherUserSessions(ctx, user.ID, SessionToken(c)); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// ListIdentities returns the linked OAuth providers.
func (s *Service) ListIdentities(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	rows, err := s.Q.ListIdentitiesForUser(c.Request().Context(), user.ID)
	if err != nil {
		return err
	}
	out := make([]api.LinkedIdentity, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.LinkedIdentity{
			Provider:  row.Provider,
			Email:     row.Email,
			CreatedAt: api.Timestamp(row.CreatedAt),
		})
	}
	return c.JSON(http.StatusOK, out)
}

// UnlinkIdentity removes a linked provider, refusing to strip the account of
// its last way in.
func (s *Service) UnlinkIdentity(c echo.Context) error {
	user, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	provider := c.Param("provider")
	ctx := c.Request().Context()

	count, err := s.Q.CountIdentitiesForUser(ctx, user.ID)
	if err != nil {
		return err
	}
	if user.PasswordHash == nil && count <= 1 {
		return apperr.Conflict("link another sign-in method before unlinking this one")
	}
	affected, err := s.Q.DeleteIdentity(ctx, user.ID, provider)
	if err != nil {
		return err
	}
	if affected == 0 {
		return apperr.NotFound("no %s identity is linked", provider)
	}
	return c.NoContent(http.StatusNoContent)
}

// meResponse renders the caller's profile with the current avatar.
func (s *Service) meResponse(ctx context.Context, user db.User) (api.Me, error) {
	avatarID, err := s.avatarID(ctx, user.ID)
	if err != nil {
		return api.Me{}, err
	}
	return dto.Me(user, avatarID), nil
}

func (s *Service) avatarID(ctx context.Context, userID int64) (int64, error) {
	avatar, err := s.Q.GetActiveAvatar(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return avatar.ID, nil
}

func (s *Service) lookupUser(ctx context.Context, login string) (*db.User, error) {
	lower := strings.ToLower(login)
	if strings.Contains(lower, "@") {
		user, err := s.Q.GetUserByEmail(ctx, lower)
		if err == nil {
			return &user, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, nil
	}
	user, err := s.Q.GetUserByUsername(ctx, lower)
	if err == nil {
		return &user, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return nil, nil
}

func (s *Service) checkPassword(user db.User, password string) error {
	if user.PasswordHash == nil {
		return apperr.BadRequest("this account has no password")
	}
	ok, err := VerifyPassword(*user.PasswordHash, password)
	if err != nil {
		s.Log.Error("stored password hash is unusable", zapUser(user.ID))
		return apperr.BadRequest("current password is incorrect")
	}
	if !ok {
		return apperr.BadRequest("current password is incorrect")
	}
	return nil
}

// consumeToken validates and burns a single-use email token.
func (s *Service) consumeToken(ctx context.Context, plain, kind string) (db.EmailToken, error) {
	var empty db.EmailToken
	if plain == "" {
		return empty, apperr.BadRequest("that link is not valid")
	}
	sum := sha256Sum(plain)
	token, err := s.Q.GetEmailToken(ctx, sum[:], kind)
	if errors.Is(err, sql.ErrNoRows) {
		return empty, apperr.BadRequest("that link is not valid")
	}
	if err != nil {
		return empty, err
	}
	now := time.Now().Unix()
	if token.ConsumedAt != nil {
		return empty, apperr.BadRequest("that link was already used")
	}
	if token.ExpiresAt <= now {
		return empty, apperr.BadRequest("that link has expired")
	}
	affected, err := s.Q.ConsumeEmailToken(ctx, &now, token.ID)
	if err != nil {
		return empty, err
	}
	if affected == 0 {
		return empty, apperr.BadRequest("that link was already used")
	}
	return token, nil
}

// issueEmailToken creates a single-use token of the given kind, invalidating
// any outstanding token of that kind for the user.
func (s *Service) issueEmailToken(ctx context.Context, userID int64, kind string, ttl int64) (string, error) {
	now := time.Now().Unix()
	if _, err := s.Q.InvalidateEmailTokens(ctx, &now, userID, kind); err != nil {
		return "", err
	}
	plain, sum, err := randomToken()
	if err != nil {
		return "", err
	}
	if _, err := s.Q.CreateEmailToken(ctx, userID, kind, sum[:], now+ttl, now); err != nil {
		return "", err
	}
	return plain, nil
}

func (s *Service) sendVerification(ctx context.Context, user db.User) {
	token, err := s.issueEmailToken(ctx, user.ID, TokenKindVerify, verifyTokenTTL)
	if err != nil {
		s.Log.Error("issuing a verification token failed", zapUser(user.ID), zapErr(err))
		return
	}
	link := strings.TrimSuffix(s.Cfg.BaseURL, "/") + "/verify?token=" + token
	subject, body, err := verifyTemplate.render(mailData{
		DisplayName: displayNameOf(user),
		Username:    user.Username,
		Link:        link,
		BaseURL:     s.Cfg.BaseURL,
	})
	if err != nil {
		s.Log.Error("rendering the verification mail failed", zapUser(user.ID), zapErr(err))
		return
	}
	if s.Mail.Enabled() {
		if err := s.Mail.Send(ctx, user.Email, subject, body); err != nil {
			s.Log.Warn("sending the verification mail failed", zapUser(user.ID))
		}
		return
	}
	s.Log.Warn("smtp is not configured; the verification link is logged instead", zapUser(user.ID))
	s.Log.Debug("verification link", zapUser(user.ID), zapString("link", link))
}

func (s *Service) sendPasswordReset(ctx context.Context, user db.User) {
	token, err := s.issueEmailToken(ctx, user.ID, TokenKindReset, resetTokenTTL)
	if err != nil {
		s.Log.Error("issuing a reset token failed", zapUser(user.ID), zapErr(err))
		return
	}
	link := strings.TrimSuffix(s.Cfg.BaseURL, "/") + "/reset?token=" + token
	subject, body, err := resetTemplate.render(mailData{
		DisplayName: displayNameOf(user),
		Username:    user.Username,
		Link:        link,
		BaseURL:     s.Cfg.BaseURL,
	})
	if err != nil {
		s.Log.Error("rendering the reset mail failed", zapUser(user.ID), zapErr(err))
		return
	}
	if s.Mail.Enabled() {
		if err := s.Mail.Send(ctx, user.Email, subject, body); err != nil {
			s.Log.Warn("sending the reset mail failed", zapUser(user.ID))
		}
		return
	}
	s.Log.Warn("smtp is not configured; the reset link is logged instead", zapUser(user.ID))
	s.Log.Debug("reset link", zapUser(user.ID), zapString("link", link))
}

func displayNameOf(u db.User) string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Username
}

// normalizeEmail lowercases and validates an address.
func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > 254 {
		return "", apperr.BadRequest("enter a valid email address")
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || !strings.Contains(email, ".") {
		return "", apperr.BadRequest("enter a valid email address")
	}
	return email, nil
}

// isUniqueViolation reports whether an error is a UNIQUE constraint failure.
// modernc's driver reports it as SQLITE_CONSTRAINT_UNIQUE; the message check
// keeps this stable across driver versions.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func conflictOrInternal(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	if isUniqueViolation(err) {
		return apperr.Conflict(format, args...)
	}
	return err
}
