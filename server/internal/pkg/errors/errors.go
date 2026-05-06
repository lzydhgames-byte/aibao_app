// Package errors defines AppError — a unified application error type used
// across all service-layer code. AppError carries:
//   - Code:    a business-meaning category (drives HTTP status mapping)
//   - Reason:  machine-readable identifier, e.g. "child_not_found"
//   - UserMsg: user-facing message (may be Chinese)
//   - cause:   the underlying error (preserved through Wrap)
//
// Service layer returns AppError; API layer detects it via AsAppError and
// translates to JSON. This keeps service code free of HTTP framework deps.
package errors

import (
	stderr "errors"
	"fmt"
	"net/http"
)

// Code categorizes the kind of failure. Each code maps to one HTTP status.
type Code int

const (
	// CodeInvalidArgument indicates the caller supplied bad input. Maps to 400.
	CodeInvalidArgument Code = iota + 1
	// CodeUnauthenticated indicates missing/invalid credentials. Maps to 401.
	CodeUnauthenticated
	// CodePermissionDenied indicates the caller is authenticated but lacks
	// permission for the action. Maps to 403.
	CodePermissionDenied
	// CodeNotFound indicates the requested resource does not exist. Maps to 404.
	CodeNotFound
	// CodeRateLimited indicates the caller is being throttled. Maps to 429.
	CodeRateLimited
	// CodeBudgetExceeded indicates a resource budget has been exhausted
	// (e.g. daily LLM token budget). Maps to 503 to signal "try later".
	CodeBudgetExceeded
	// CodeInternal indicates an unexpected server error. Maps to 500.
	CodeInternal
)

// AppError is the unified application error used by the service layer.
type AppError struct {
	Code    Code
	Reason  string // machine-readable, e.g. "child_not_found"
	UserMsg string // user-facing, may be Chinese
	cause   error
}

// New constructs a fresh AppError without a wrapped cause.
func New(code Code, reason, userMsg string) *AppError {
	return &AppError{Code: code, Reason: reason, UserMsg: userMsg}
}

// Wrap constructs an AppError that wraps an underlying cause.
// The cause is preserved for errors.Is/As traversal.
func Wrap(cause error, code Code, reason, userMsg string) *AppError {
	return &AppError{Code: code, Reason: reason, UserMsg: userMsg, cause: cause}
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Reason, e.UserMsg, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Reason, e.UserMsg)
}

// Unwrap returns the wrapped cause so errors.Is/As can traverse the chain.
func (e *AppError) Unwrap() error { return e.cause }

// HTTPStatus returns the HTTP status code corresponding to e.Code.
func (e *AppError) HTTPStatus() int {
	switch e.Code {
	case CodeInvalidArgument:
		return http.StatusBadRequest
	case CodeUnauthenticated:
		return http.StatusUnauthorized
	case CodePermissionDenied:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeRateLimited:
		return http.StatusTooManyRequests
	case CodeBudgetExceeded:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// AsAppError extracts an *AppError from anywhere in err's chain, if present.
func AsAppError(err error) (*AppError, bool) {
	var e *AppError
	if stderr.As(err, &e) {
		return e, true
	}
	return nil, false
}
