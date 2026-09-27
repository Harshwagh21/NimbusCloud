package apierr_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/Harshwagh21/NimbusCloud/internal/platform/apierr"
)

func TestStatusMapsEveryKindToItsHTTPCode(t *testing.T) {
	cases := []struct {
		err        error
		wantStatus int
	}{
		{apierr.BadRequest("INVALID_BODY", "malformed request body"), http.StatusBadRequest},
		{apierr.Unauthorized("NO_TOKEN", "authentication required"), http.StatusUnauthorized},
		{apierr.Forbidden("NOT_ALLOWED", "action not permitted"), http.StatusForbidden},
		{apierr.NotFound("FILE_NOT_FOUND", "file not found"), http.StatusNotFound},
		{apierr.Conflict("NAME_TAKEN", "a file with that name exists"), http.StatusConflict},
		{apierr.Unprocessable("SIZE_MISMATCH", "stored size differs"), http.StatusUnprocessableEntity},
		{apierr.RateLimited("TOO_MANY", "slow down"), http.StatusTooManyRequests},
		{apierr.Internal(errors.New("connection reset")), http.StatusInternalServerError},
	}

	for _, tc := range cases {
		if got := apierr.Status(tc.err); got != tc.wantStatus {
			t.Errorf("%v: want status %d, got %d", tc.err, tc.wantStatus, got)
		}
	}
}

// Anything that is not an *apierr.Error is an unexpected failure, so it must not leak
// its message to the client as if it were a handled case.
func TestUnrecognisedErrorsBecomeOpaqueInternalErrors(t *testing.T) {
	raw := errors.New("pq: duplicate key value violates unique constraint \"users_email_key\"")

	if got := apierr.Status(raw); got != http.StatusInternalServerError {
		t.Errorf("want status 500 for an unrecognised error, got %d", got)
	}

	converted := apierr.From(raw)
	if strings.Contains(converted.Message, "users_email_key") {
		t.Errorf("client message must not expose internal detail, got %q", converted.Message)
	}
	if converted.Code == "" {
		t.Error("converted errors must still carry a machine-readable code")
	}
}

func TestInternalKeepsTheCauseForLoggingButNotForTheClient(t *testing.T) {
	cause := errors.New("dial tcp 127.0.0.1:5433: connect: connection refused")
	err := apierr.Internal(cause)

	if !errors.Is(err, cause) {
		t.Error("the original cause must remain unwrappable so it can be logged")
	}
	if strings.Contains(err.Message, "connection refused") {
		t.Errorf("client message must not expose the cause, got %q", err.Message)
	}
}

// Services return typed errors; callers identify them with errors.As, not string matching.
func TestErrorsAreIdentifiableWithErrorsAs(t *testing.T) {
	wrapped := errors.Join(errors.New("while loading folder"), apierr.NotFound("FOLDER_NOT_FOUND", "folder not found"))

	var target *apierr.Error
	if !errors.As(wrapped, &target) {
		t.Fatal("expected to recover the *apierr.Error from a wrapped error")
	}
	if target.Code != "FOLDER_NOT_FOUND" {
		t.Errorf("want code FOLDER_NOT_FOUND, got %q", target.Code)
	}
}
