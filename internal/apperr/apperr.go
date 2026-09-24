// Package apperr defines the HTTP error contract shared by every handler
// package. Handlers return *apperr.Error values; internal/server turns them
// into the JSON envelope and nothing else.
package apperr

import (
	"fmt"
	"net/http"
)

// Machine-readable error codes. They are part of the public HTTP contract.
const (
	CodeInvalidRequest    = "invalid_request"
	CodeUnauthorized      = "unauthorized"
	CodeForbidden         = "forbidden"
	CodeNotFound          = "not_found"
	CodeConflict          = "conflict"
	CodeDuplicateActivity = "duplicate_activity"
	CodeRateLimited       = "rate_limited"
	CodeInternal          = "internal"
)

// Error is a handler error carrying its own HTTP status and machine code.
type Error struct {
	Code    string
	Status  int
	Message string
	// ActivityID is an optional machine-readable detail carried by
	// duplicate_activity responses, so the upload UI can link to the copy that
	// already exists. Zero means "absent".
	ActivityID int64
}

func (e *Error) Error() string { return e.Message }

// New builds an Error.
func New(code string, status int, message string) *Error {
	return &Error{Code: code, Status: status, Message: message}
}

// Errorf builds an Error with a formatted human message.
func Errorf(code string, status int, format string, args ...any) *Error {
	return &Error{Code: code, Status: status, Message: fmt.Sprintf(format, args...)}
}

// BadRequest is a 400 with the invalid_request code.
func BadRequest(format string, args ...any) *Error {
	return Errorf(CodeInvalidRequest, http.StatusBadRequest, format, args...)
}

// Conflict is a 409.
func Conflict(format string, args ...any) *Error {
	return Errorf(CodeConflict, http.StatusConflict, format, args...)
}

// NotFound is a 404.
func NotFound(format string, args ...any) *Error {
	return Errorf(CodeNotFound, http.StatusNotFound, format, args...)
}

// Forbidden is a 403.
func Forbidden(format string, args ...any) *Error {
	return Errorf(CodeForbidden, http.StatusForbidden, format, args...)
}

// The sentinels are shared immutable values.
var (
	ErrUnauthorized    = New(CodeUnauthorized, http.StatusUnauthorized, "authentication required")
	ErrForbidden       = New(CodeForbidden, http.StatusForbidden, "not allowed")
	ErrNotFound        = New(CodeNotFound, http.StatusNotFound, "not found")
	ErrInvalidRequest  = New(CodeInvalidRequest, http.StatusBadRequest, "invalid request")
	ErrConflict        = New(CodeConflict, http.StatusConflict, "conflict")
	ErrRateLimited     = New(CodeRateLimited, http.StatusTooManyRequests, "too many requests")
	ErrInternal        = New(CodeInternal, http.StatusInternalServerError, "internal error")
	ErrDuplicateUpload = New(CodeDuplicateActivity, http.StatusConflict, "activity already imported")
)
