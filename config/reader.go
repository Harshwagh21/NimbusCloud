package config

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// reader pulls typed values out of the environment while accumulating problems, so that
// Load can report every misconfiguration at once instead of failing on the first one.
//
// An empty value is treated as absent. That keeps behaviour identical whether a variable is
// unset or set to an empty string, which is what shells and container platforms actually do.
type reader struct {
	problems []error
}

func (r *reader) err() error {
	if len(r.problems) == 0 {
		return nil
	}
	return fmt.Errorf("invalid configuration: %w", errors.Join(r.problems...))
}

func (r *reader) fail(key, format string, args ...any) {
	r.problems = append(r.problems, fmt.Errorf("%s "+format, append([]any{key}, args...)...))
}

func (r *reader) optional(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func (r *reader) required(key string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		r.fail(key, "is required")
	}
	return value
}

func (r *reader) oneOf(key, fallback string, allowed ...string) string {
	value := r.optional(key, fallback)
	if slices.Contains(allowed, value) {
		return value
	}
	r.fail(key, "must be one of [%s], got %q", strings.Join(allowed, ", "), value)
	return fallback
}

func (r *reader) duration(key string, fallback time.Duration) time.Duration {
	raw := r.optional(key, "")
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		r.fail(key, "must be a positive duration such as 15m or 720h, got %q", raw)
		return fallback
	}
	return value
}

func (r *reader) positiveInt(key string, fallback int64) int64 {
	raw := r.optional(key, "")
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		r.fail(key, "must be a positive integer, got %q", raw)
		return fallback
	}
	return value
}

// positiveInt32 parses a pool-size style setting. bitSize 32 rejects values that cannot
// become int32, which is what pgxpool.Config.MaxConns requires.
func (r *reader) positiveInt32(key string, fallback int32) int32 {
	raw := r.optional(key, "")
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || value <= 0 {
		r.fail(key, "must be a positive 32-bit integer, got %q", raw)
		return fallback
	}
	return int32(value)
}

func (r *reader) boolean(key string, fallback bool) bool {
	raw := r.optional(key, "")
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		r.fail(key, "must be true or false, got %q", raw)
		return fallback
	}
	return value
}

// list splits a comma-separated value, dropping blanks so a trailing comma is harmless.
func (r *reader) list(key, fallback string) []string {
	raw := strings.Split(r.optional(key, fallback), ",")
	values := make([]string, 0, len(raw))
	for _, item := range raw {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

// requireStrongProductionSecret stops a development-length signing key from reaching
// production, where a guessable secret means forgeable access tokens.
func (r *reader) requireStrongProductionSecret(cfg Config) {
	if cfg.AppEnv != EnvProduction || cfg.JWTSecret == "" {
		return
	}
	if len(cfg.JWTSecret) < minProductionSecretLength {
		r.fail("JWT_SECRET", "must be at least %d characters in production", minProductionSecretLength)
	}
}
