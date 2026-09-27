package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Harshwagh21/NimbusCloud/config"
)

// Every key the loader reads. Cleared before each test so a value in the developer's real
// environment cannot make a test pass or fail by accident.
var allKeys = []string{
	"APP_ENV", "HTTP_PORT", "LOG_LEVEL",
	"DATABASE_URL", "DATABASE_MAX_CONNS",
	"JWT_SECRET", "ACCESS_TOKEN_TTL", "REFRESH_TOKEN_TTL",
	"S3_ENDPOINT", "S3_REGION", "S3_BUCKET", "S3_ACCESS_KEY", "S3_SECRET_KEY",
	"S3_FORCE_PATH_STYLE", "S3_PRESIGN_TTL",
	"STORAGE_MODE", "FREE_STORAGE_LIMIT_BYTES", "CORS_ALLOWED_ORIGINS",
}

func withEnv(t *testing.T, overrides map[string]string) {
	t.Helper()
	for _, key := range allKeys {
		t.Setenv(key, "")
	}
	for key, value := range overrides {
		t.Setenv(key, value)
	}
}

func requiredEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL":  "postgres://user:pass@localhost:5433/nimbus?sslmode=disable",
		"JWT_SECRET":    "a-development-secret-that-is-long-enough",
		"S3_ENDPOINT":   "http://localhost:9000",
		"S3_BUCKET":     "nimbus-files",
		"S3_ACCESS_KEY": "nimbus_minio",
		"S3_SECRET_KEY": "nimbus_minio_dev_password",
	}
}

func TestLoadAppliesDocumentedDefaultsWhenOnlyRequiredVariablesAreSet(t *testing.T) {
	withEnv(t, requiredEnv())

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected a valid config, got error: %v", err)
	}

	assertEqual(t, "APP_ENV", config.EnvDevelopment, cfg.AppEnv)
	assertEqual(t, "HTTP_PORT", "8080", cfg.HTTPPort)
	assertEqual(t, "LOG_LEVEL", "info", cfg.LogLevel)
	assertEqual(t, "DATABASE_MAX_CONNS", int32(10), cfg.DatabaseMaxConns)
	assertEqual(t, "ACCESS_TOKEN_TTL", 15*time.Minute, cfg.AccessTokenTTL)
	assertEqual(t, "REFRESH_TOKEN_TTL", 720*time.Hour, cfg.RefreshTokenTTL)
	assertEqual(t, "S3_REGION", "us-east-1", cfg.Storage.Region)
	assertEqual(t, "S3_FORCE_PATH_STYLE", true, cfg.Storage.ForcePathStyle)
	assertEqual(t, "S3_PRESIGN_TTL", 15*time.Minute, cfg.Storage.PresignTTL)
	assertEqual(t, "STORAGE_MODE", config.StorageModeFree, cfg.Storage.Mode)
	assertEqual(t, "FREE_STORAGE_LIMIT_BYTES", int64(10_737_418_240), cfg.Storage.FreeLimitBytes)
}

// One restart should reveal every misconfiguration, not just the first one.
func TestLoadReportsEveryMissingRequiredVariableAtOnce(t *testing.T) {
	withEnv(t, nil)

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected an error when no required variables are set")
	}

	for _, key := range []string{"DATABASE_URL", "JWT_SECRET", "S3_ENDPOINT", "S3_BUCKET", "S3_ACCESS_KEY", "S3_SECRET_KEY"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error should name the missing variable %q, got: %v", key, err)
		}
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name      string
		overrides map[string]string
		wantInErr string
	}{
		{"unknown storage mode", map[string]string{"STORAGE_MODE": "unlimited"}, "STORAGE_MODE"},
		{"malformed duration", map[string]string{"ACCESS_TOKEN_TTL": "fifteen minutes"}, "ACCESS_TOKEN_TTL"},
		{"non numeric connection count", map[string]string{"DATABASE_MAX_CONNS": "many"}, "DATABASE_MAX_CONNS"},
		{"zero connection count", map[string]string{"DATABASE_MAX_CONNS": "0"}, "DATABASE_MAX_CONNS"},
		{"negative storage limit", map[string]string{"FREE_STORAGE_LIMIT_BYTES": "-1"}, "FREE_STORAGE_LIMIT_BYTES"},
		{"unknown app env", map[string]string{"APP_ENV": "staging-ish"}, "APP_ENV"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := requiredEnv()
			for key, value := range tc.overrides {
				env[key] = value
			}
			withEnv(t, env)

			if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), tc.wantInErr) {
				t.Fatalf("expected an error naming %q, got: %v", tc.wantInErr, err)
			}
		})
	}
}

// A development-length secret must never reach production unnoticed.
func TestLoadRequiresAStrongJWTSecretInProduction(t *testing.T) {
	env := requiredEnv()
	env["APP_ENV"] = config.EnvProduction
	env["JWT_SECRET"] = "too-short"
	withEnv(t, env)

	_, err := config.Load()
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("expected a JWT_SECRET strength error in production, got: %v", err)
	}
}

func TestLoadParsesCORSOriginsAndIgnoresWhitespaceAndBlanks(t *testing.T) {
	env := requiredEnv()
	env["CORS_ALLOWED_ORIGINS"] = " http://localhost:3000 , ,https://nimbus.example "
	withEnv(t, env)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"http://localhost:3000", "https://nimbus.example"}
	if len(cfg.CORSAllowedOrigins) != len(want) {
		t.Fatalf("expected %d origins, got %v", len(want), cfg.CORSAllowedOrigins)
	}
	for i, origin := range want {
		assertEqual(t, "origin", origin, cfg.CORSAllowedOrigins[i])
	}
}

func TestEnforcesQuotaOnlyInFreeMode(t *testing.T) {
	cases := map[string]bool{config.StorageModeFree: true, config.StorageModePremium: false}

	for mode, wantEnforced := range cases {
		t.Run(mode, func(t *testing.T) {
			env := requiredEnv()
			env["STORAGE_MODE"] = mode
			withEnv(t, env)

			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertEqual(t, "EnforcesQuota", wantEnforced, cfg.Storage.EnforcesQuota())
		})
	}
}

func assertEqual[T comparable](t *testing.T, name string, want, got T) {
	t.Helper()
	if want != got {
		t.Errorf("%s: want %v, got %v", name, want, got)
	}
}
