package server

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/markbeep/hyl/internal/apperr"
)

// contextWithTimeout derives a bounded context from the request.
func contextWithTimeout(c echo.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request().Context(), d)
}

// Errorf builds an apperr.Error with the internal code.
func Errorf(format string, args ...any) error {
	return apperr.Errorf(apperr.CodeInternal, http.StatusInternalServerError, format, args...)
}

// validator keeps echo from pulling in its own validator dependency; all
// request validation is explicit in the handlers.
type validator struct{}

func (validator) Validate(any) error { return nil }
