// Package apierr defines the one error type services return and the single mapping from
// those errors to HTTP status codes.
//
// Handlers never decide a status code themselves. That keeps behaviour consistent across
// every endpoint — in particular the deliberate choice to answer 404 rather than 403 for a
// resource owned by somebody else, so the API never confirms that it exists.
package apierr

import (
	"errors"
	"fmt"
	"net/http"
)

type Kind uint8

const (
	KindInternal Kind = iota
	KindBadRequest
	KindUnauthorized
	KindForbidden
	KindNotFound
	KindConflict
	KindUnprocessable
	KindRateLimited
)

const (
	CodeInternal = "INTERNAL_ERROR"
	// Returned to clients in place of any unrecognised error, so internal details such as
	// SQL constraint names never reach the outside world.
	genericInternalMessage = "an unexpected error occurred"
)

var statusByKind = map[Kind]int{
	KindInternal:      http.StatusInternalServerError,
	KindBadRequest:    http.StatusBadRequest,
	KindUnauthorized:  http.StatusUnauthorized,
	KindForbidden:     http.StatusForbidden,
	KindNotFound:      http.StatusNotFound,
	KindConflict:      http.StatusConflict,
	KindUnprocessable: http.StatusUnprocessableEntity,
	KindRateLimited:   http.StatusTooManyRequests,
}

// Error carries a stable machine-readable Code and a Message that is safe to return to a
// client. The cause is kept for logs only and is never serialised.
type Error struct {
	Kind    Kind
	Code    string
	Message string
	cause   error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.cause }

// WithCause attaches an internal cause for logging without changing the client response.
func (e *Error) WithCause(cause error) *Error {
	e.cause = cause
	return e
}

func newError(kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

func BadRequest(code, message string) *Error { return newError(KindBadRequest, code, message) }
func Unauthorized(code, message string) *Error {
	return newError(KindUnauthorized, code, message)
}
func Forbidden(code, message string) *Error { return newError(KindForbidden, code, message) }
func NotFound(code, message string) *Error  { return newError(KindNotFound, code, message) }
func Conflict(code, message string) *Error  { return newError(KindConflict, code, message) }
func Unprocessable(code, message string) *Error {
	return newError(KindUnprocessable, code, message)
}
func RateLimited(code, message string) *Error { return newError(KindRateLimited, code, message) }

// Internal wraps an unexpected failure. The cause stays available for logging via errors.Is
// and errors.As, while the client only ever sees the generic message.
func Internal(cause error) *Error {
	return newError(KindInternal, CodeInternal, genericInternalMessage).WithCause(cause)
}

// From recovers the *Error from anywhere in an error chain. Anything unrecognised is an
// unexpected failure and becomes an opaque internal error.
func From(err error) *Error {
	var typed *Error
	if errors.As(err, &typed) {
		return typed
	}
	return Internal(err)
}

func Status(err error) int {
	return statusByKind[From(err).Kind]
}
