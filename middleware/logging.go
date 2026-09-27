package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// silentPaths are polled constantly by the platform's probes and would otherwise bury real
// traffic in the logs.
var silentPaths = map[string]struct{}{
	"/healthz": {},
	"/readyz":  {},
}

// Logger emits one structured line per request. Severity follows the status code so that an
// error-rate alert can be built from the log stream alone.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		if _, silent := silentPaths[c.Request.URL.Path]; silent {
			return
		}
		logRequest(c, time.Since(start))
	}
}

func logRequest(c *gin.Context, elapsed time.Duration) {
	status := c.Writer.Status()
	attributes := []any{
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
		"status", status,
		"duration_ms", elapsed.Milliseconds(),
		"request_id", RequestIDFrom(c.Request.Context()),
	}

	switch {
	case status >= 500:
		slog.Error("request", attributes...)
	case status >= 400:
		slog.Warn("request", attributes...)
	default:
		slog.Info("request", attributes...)
	}
}
