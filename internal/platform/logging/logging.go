// Package logging configures the process-wide structured logger.
package logging

import (
	"log/slog"
	"os"

	"github.com/Harshwagh21/NimbusCloud/config"
)

var levels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

// Setup installs the default logger: JSON in production so a log platform can index the
// fields, and human-readable text locally.
func Setup(cfg config.Config) {
	options := &slog.HandlerOptions{Level: levels[cfg.LogLevel]}

	var handler slog.Handler = slog.NewTextHandler(os.Stdout, options)
	if cfg.IsProduction() {
		handler = slog.NewJSONHandler(os.Stdout, options)
	}

	slog.SetDefault(slog.New(handler))
}
