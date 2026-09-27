// Package config loads and validates every setting the API needs from the environment.
//
// The process exits at startup if anything is missing or malformed, so a misconfiguration
// surfaces immediately rather than on the first request that happens to need the value.
package config

import "time"

const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
	EnvTest        = "test"

	// StorageModeFree applies the application-level storage cap. StorageModePremium
	// removes it and lets the object store's own billing apply.
	StorageModeFree    = "free"
	StorageModePremium = "premium"

	defaultFreeLimitBytes     = 10_737_418_240 // 10 GiB, matching the R2 free allowance
	minProductionSecretLength = 32
)

// Config is the fully validated configuration. Nothing in the codebase reads os.Getenv
// directly, so every setting is visible in one place.
type Config struct {
	AppEnv   string
	HTTPPort string
	LogLevel string

	DatabaseURL      string
	DatabaseMaxConns int32

	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration

	Storage            Storage
	CORSAllowedOrigins []string
}

// Storage describes any S3-compatible backend: MinIO locally, Cloudflare R2 in production.
type Storage struct {
	Endpoint       string
	Region         string
	Bucket         string
	AccessKey      string
	SecretKey      string
	ForcePathStyle bool
	PresignTTL     time.Duration
	Mode           string
	FreeLimitBytes int64
}

func (c Config) IsProduction() bool { return c.AppEnv == EnvProduction }

// EnforcesQuota reports whether the application-level storage cap applies.
func (s Storage) EnforcesQuota() bool { return s.Mode == StorageModeFree }

// Load reads the environment and returns a validated Config. Every problem found is
// reported together, so one restart reveals all of them.
func Load() (Config, error) {
	env := &reader{}

	cfg := Config{
		AppEnv:   env.oneOf("APP_ENV", EnvDevelopment, EnvDevelopment, EnvProduction, EnvTest),
		HTTPPort: env.optional("HTTP_PORT", "8080"),
		LogLevel: env.oneOf("LOG_LEVEL", "info", "debug", "info", "warn", "error"),

		DatabaseURL:      env.required("DATABASE_URL"),
		DatabaseMaxConns: int32(env.positiveInt("DATABASE_MAX_CONNS", 10)),

		JWTSecret:       env.required("JWT_SECRET"),
		AccessTokenTTL:  env.duration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL: env.duration("REFRESH_TOKEN_TTL", 720*time.Hour),

		Storage:            loadStorage(env),
		CORSAllowedOrigins: env.list("CORS_ALLOWED_ORIGINS", "http://localhost:3000"),
	}

	env.requireStrongProductionSecret(cfg)
	return cfg, env.err()
}

func loadStorage(env *reader) Storage {
	return Storage{
		Endpoint:  env.required("S3_ENDPOINT"),
		Region:    env.optional("S3_REGION", "us-east-1"),
		Bucket:    env.required("S3_BUCKET"),
		AccessKey: env.required("S3_ACCESS_KEY"),
		SecretKey: env.required("S3_SECRET_KEY"),
		// MinIO requires path-style addressing; R2 accepts it.
		ForcePathStyle: env.boolean("S3_FORCE_PATH_STYLE", true),
		PresignTTL:     env.duration("S3_PRESIGN_TTL", 15*time.Minute),
		Mode:           env.oneOf("STORAGE_MODE", StorageModeFree, StorageModeFree, StorageModePremium),
		FreeLimitBytes: env.positiveInt("FREE_STORAGE_LIMIT_BYTES", defaultFreeLimitBytes),
	}
}
