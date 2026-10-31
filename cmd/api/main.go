// Command api is the Nimbus API entry point. It does nothing but wiring: load
// configuration, open dependencies, build the router, and serve until told to stop.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/Harshwagh21/NimbusCloud/config"
	authhandler "github.com/Harshwagh21/NimbusCloud/internal/handler/auth"
	"github.com/Harshwagh21/NimbusCloud/internal/health"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/database"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/logging"
	authrepo "github.com/Harshwagh21/NimbusCloud/internal/repository/auth"
	"github.com/Harshwagh21/NimbusCloud/internal/repository/user"
	"github.com/Harshwagh21/NimbusCloud/internal/server"
	authsvc "github.com/Harshwagh21/NimbusCloud/internal/service/auth"
	"github.com/Harshwagh21/NimbusCloud/middleware"
)

// version is injected at build time with -ldflags and reported by the probes.
var version = "dev"

const (
	readHeaderTimeout  = 10 * time.Second
	idleTimeout        = 60 * time.Second
	shutdownTimeout    = 10 * time.Second
	loginAttemptLimit  = 10
	loginAttemptWindow = time.Minute
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// A missing .env is expected in production, where real environment variables are used.
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logging.Setup(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	router, err := buildRouter(cfg, pool)
	if err != nil {
		return err
	}

	return serve(ctx, cfg, router)
}

func buildRouter(cfg config.Config, pool *pgxpool.Pool) (http.Handler, error) {
	service, err := authsvc.New(user.NewRepository(pool), authrepo.NewRepository(pool), authsvc.Params{
		JWTSecret:       cfg.JWTSecret,
		AccessTokenTTL:  cfg.AccessTokenTTL,
		RefreshTokenTTL: cfg.RefreshTokenTTL,
		StorageLimit:    cfg.Storage.FreeLimitBytes,
	})
	if err != nil {
		return nil, err
	}
	limiter := middleware.NewLimiter(loginAttemptLimit, loginAttemptWindow)
	return server.NewRouter(server.Dependencies{
		Config:           cfg,
		Health:           health.New(version, database.NewPoolChecker(pool)),
		Auth:             authhandler.New(service, cfg.RefreshTokenTTL),
		ParseAccessToken: service.ParseAccessToken,
		LimitAuth:        limiter.Handler(),
	}), nil
}

// serve runs the server until the context is cancelled, then drains in-flight requests so a
// deploy or restart does not sever an active connection.
func serve(ctx context.Context, cfg config.Config, handler http.Handler) error {
	server := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: handler,
		// Bounded header reads; an unbounded one invites a slow-header denial of service.
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
	}

	failed := make(chan error, 1)
	go func() { failed <- server.ListenAndServe() }()
	slog.Info("api listening", "port", cfg.HTTPPort, "env", cfg.AppEnv, "version", version)

	select {
	case err := <-failed:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		slog.Info("shutdown signal received, draining requests")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
