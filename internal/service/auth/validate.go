package auth

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Harshwagh21/NimbusCloud/internal/platform/apierr"
)

func invalidCredentials() error {
	return apierr.Unauthorized("INVALID_CREDENTIALS", "email or password is incorrect")
}

func invalidRefresh() error {
	return apierr.Unauthorized("INVALID_REFRESH_TOKEN", "session is no longer valid")
}

func refreshReused() error {
	return apierr.Unauthorized("REFRESH_REUSED", "session is no longer valid")
}

func emailTaken() error {
	return apierr.Conflict("EMAIL_TAKEN", "an account with that email already exists")
}

func unauthenticated() error {
	return apierr.Unauthorized("UNAUTHENTICATED", "authentication is required")
}

func validateRegister(email, name, password string) error {
	if err := validateEmail(email); err != nil {
		return err
	}
	if err := validateName(name); err != nil {
		return err
	}
	return validatePassword(password)
}

func validateLogin(email, password string) error {
	if err := validateEmail(email); err != nil {
		return err
	}
	if password == "" {
		return apierr.BadRequest("VALIDATION_ERROR", "password is required")
	}
	return nil
}

func validateName(name string) error {
	if name == "" {
		return apierr.BadRequest("VALIDATION_ERROR", "name is required")
	}
	if len(name) > 100 {
		return apierr.BadRequest("VALIDATION_ERROR", "name is too long")
	}
	return nil
}

func validateEmail(email string) error {
	if !emailLooksValid(email) {
		return apierr.BadRequest("VALIDATION_ERROR", "a valid email is required")
	}
	return nil
}

func emailLooksValid(email string) bool {
	if email == "" || len(email) > 254 || strings.ContainsAny(email, " \t\r\n") {
		return false
	}
	at := strings.IndexByte(email, '@')
	if at <= 0 || at != strings.LastIndexByte(email, '@') || at >= len(email)-1 {
		return false
	}
	return strings.Contains(email[at+1:], ".")
}

func validatePassword(password string) error {
	if password == "" {
		return apierr.BadRequest("VALIDATION_ERROR", "password is required")
	}
	if len(password) < minPasswordLen || len(password) > maxPasswordLen {
		return apierr.Unprocessable("WEAK_PASSWORD", "password must be 8 to 72 characters")
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
