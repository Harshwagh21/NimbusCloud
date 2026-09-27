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

	"github.com/joho/godotenv"

	"github.com/Harshwagh21/NimbusCloud/config"
	"github.com/Harshwagh21/NimbusCloud/internal/health"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/database"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/logging"
	"github.com/Harshwagh21/NimbusCloud/internal/server"
)

// version is injected at build time with -ldflags and reported by the probes.
var version = "dev"

const (
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 10 * time.Second
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

	router := server.NewRouter(server.Dependencies{
		Config: cfg,
		Health: health.New(version, database.NewPoolChecker(pool)),
	})

	return serve(ctx, cfg, router)
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
