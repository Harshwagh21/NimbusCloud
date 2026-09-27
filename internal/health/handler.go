// Package health serves the liveness and readiness probes.
//
// The two are deliberately different. Liveness answers "is this process working", so a
// failing dependency must not fail it — otherwise an unreachable database would make the
// platform restart a perfectly healthy container, repeatedly, for no benefit. Readiness
// answers "should traffic be sent here", which does depend on every dependency.
package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Checker is implemented by anything the API needs in order to serve traffic. New
// dependencies register themselves rather than being hardcoded here, so adding object
// storage in a later phase does not change this package.
type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

const (
	defaultCheckTimeout = 2 * time.Second
	resultOK            = "ok"
	// Probe output is public, so a failing check reports only that it is unavailable.
	// The underlying reason goes to the logs.
	resultUnavailable = "unavailable"
)

type Handler struct {
	version  string
	checkers []Checker

	// CheckTimeout bounds how long the readiness probe waits for its dependencies.
	CheckTimeout time.Duration
}

func New(version string, checkers ...Checker) *Handler {
	return &Handler{version: version, checkers: checkers, CheckTimeout: defaultCheckTimeout}
}

// Live reports that the process is up. It intentionally checks nothing else.
func (h *Handler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": resultOK, "version": h.version})
}

// Ready reports 200 only when every dependency responds, and 503 otherwise.
func (h *Handler) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.CheckTimeout)
	defer cancel()

	results, healthy := h.runChecks(ctx)

	status, state := http.StatusOK, resultOK
	if !healthy {
		status, state = http.StatusServiceUnavailable, "degraded"
	}
	c.JSON(status, gin.H{"status": state, "version": h.version, "checks": results})
}

// runChecks probes every dependency concurrently, so the probe costs the slowest check
// rather than the sum of all of them.
func (h *Handler) runChecks(ctx context.Context) (map[string]string, bool) {
	type outcome struct{ name, result string }

	outcomes := make(chan outcome, len(h.checkers))
	for _, checker := range h.checkers {
		go func() {
			if err := checker.Check(ctx); err != nil {
				slog.ErrorContext(ctx, "readiness check failed", "dependency", checker.Name(), "error", err)
				outcomes <- outcome{checker.Name(), resultUnavailable}
				return
			}
			outcomes <- outcome{checker.Name(), resultOK}
		}()
	}

	results, healthy := make(map[string]string, len(h.checkers)), true
	for range h.checkers {
		result := <-outcomes
		results[result.name] = result.result
		healthy = healthy && result.result == resultOK
	}
	return results, healthy
}
