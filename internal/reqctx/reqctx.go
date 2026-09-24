// Package reqctx carries the authenticated user through an echo request.
//
// It exists so handler packages can read the current user without importing
// internal/server (which imports them).
package reqctx

import (
	"github.com/labstack/echo/v4"

	"github.com/markbeep/hyl/internal/apperr"
	"github.com/markbeep/hyl/internal/db"
)

const userKey = "hyl.current_user"

// SetUser stores the resolved user on the request.
func SetUser(c echo.Context, u *db.User) { c.Set(userKey, u) }

// User returns the resolved user, or nil for an anonymous request.
func User(c echo.Context) *db.User {
	u, _ := c.Get(userKey).(*db.User)
	return u
}

// UserID returns the resolved user's id, or 0 for an anonymous request.
func UserID(c echo.Context) int64 {
	if u := User(c); u != nil {
		return u.ID
	}
	return 0
}

// RequireUser returns the resolved user or a 401 error.
func RequireUser(c echo.Context) (*db.User, error) {
	if u := User(c); u != nil {
		return u, nil
	}
	return nil, apperr.ErrUnauthorized
}
