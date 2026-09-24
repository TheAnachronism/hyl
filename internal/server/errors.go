package server

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/markbeep/hyl/internal/api"
	"github.com/markbeep/hyl/internal/apperr"
)

// ErrorHandler is the single place where an error becomes a JSON envelope.
// Unknown errors are logged and reported as `internal`; nothing that reaches a
// client ever contains SQL or file system detail.
func ErrorHandler(log *zap.Logger) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}

		var appErr *apperr.Error
		var httpErr *echo.HTTPError
		status := http.StatusInternalServerError
		code := apperr.CodeInternal
		message := "internal error"
		var activityID *int64

		switch {
		case errors.As(err, &appErr):
			status, code, message = appErr.Status, appErr.Code, appErr.Message
			if appErr.ActivityID != 0 {
				activityID = &appErr.ActivityID
			}
		case errors.As(err, &httpErr):
			status = httpErr.Code
			code, message = codeForStatus(status)
			if msg, ok := httpErr.Message.(string); ok && msg != "" {
				message = msg
			}
		}

		if status >= 500 {
			log.Error("request failed", zap.Error(err), zap.String("path", c.Request().URL.Path))
		}

		if c.Request().Method == http.MethodHead {
			_ = c.NoContent(status)
			return
		}
		_ = c.JSON(status, api.ErrorResponse{Error: api.ErrorBody{Code: code, Message: message, ActivityID: activityID}})
	}
}

func codeForStatus(status int) (string, string) {
	switch status {
	case http.StatusUnauthorized:
		return apperr.CodeUnauthorized, "authentication required"
	case http.StatusForbidden:
		return apperr.CodeForbidden, "forbidden"
	case http.StatusNotFound:
		return apperr.CodeNotFound, "not found"
	case http.StatusConflict:
		return apperr.CodeConflict, "conflict"
	case http.StatusTooManyRequests:
		return apperr.CodeRateLimited, "too many requests"
	case http.StatusBadRequest, http.StatusUnsupportedMediaType, http.StatusRequestEntityTooLarge:
		return apperr.CodeInvalidRequest, "invalid request"
	default:
		if status >= 500 {
			return apperr.CodeInternal, "internal error"
		}
		return http.StatusText(status), http.StatusText(status)
	}
}
