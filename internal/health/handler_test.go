package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Harshwagh21/NimbusCloud/internal/health"
)

type stubChecker struct {
	name  string
	err   error
	delay time.Duration
}

func (s stubChecker) Name() string { return s.name }

func (s stubChecker) Check(ctx context.Context) error {
	if s.delay == 0 {
		return s.err
	}
	select {
	case <-time.After(s.delay):
		return s.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

func call(t *testing.T, handler gin.HandlerFunc) (int, map[string]any) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handler(ctx)

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response was not valid JSON: %v (%s)", err, recorder.Body.String())
	}
	return recorder.Code, body
}

// Liveness answers "is the process running", so a broken dependency must not fail it —
// otherwise an unreachable database would make the orchestrator restart a healthy process.
func TestLiveReturnsOKEvenWhenADependencyIsDown(t *testing.T) {
	handler := health.New("test", stubChecker{name: "postgres", err: errors.New("down")})

	status, body := call(t, handler.Live)

	if status != http.StatusOK {
		t.Errorf("want status 200, got %d", status)
	}
	if body["status"] != "ok" {
		t.Errorf(`want status "ok", got %v`, body["status"])
	}
}

func TestReadyReturnsOKWhenEveryDependencyIsHealthy(t *testing.T) {
	handler := health.New("test",
		stubChecker{name: "postgres"},
		stubChecker{name: "storage"},
	)

	status, body := call(t, handler.Ready)

	if status != http.StatusOK {
		t.Fatalf("want status 200, got %d (%v)", status, body)
	}
	checks, ok := body["checks"].(map[string]any)
	if !ok {
		t.Fatalf("expected a checks object, got %v", body["checks"])
	}
	for _, name := range []string{"postgres", "storage"} {
		if checks[name] != "ok" {
			t.Errorf("want %s to report ok, got %v", name, checks[name])
		}
	}
}

func TestReadyReturns503AndNamesTheFailingDependency(t *testing.T) {
	handler := health.New("test",
		stubChecker{name: "postgres", err: errors.New("connection refused")},
		stubChecker{name: "storage"},
	)

	status, body := call(t, handler.Ready)

	if status != http.StatusServiceUnavailable {
		t.Fatalf("want status 503, got %d", status)
	}
	checks := body["checks"].(map[string]any)
	if checks["postgres"] == "ok" {
		t.Error("the failing dependency must not report ok")
	}
	if checks["storage"] != "ok" {
		t.Errorf("the healthy dependency should still report ok, got %v", checks["storage"])
	}
}

// A hanging dependency must not hang the readiness probe.
func TestReadyFailsFastWhenADependencyHangs(t *testing.T) {
	handler := health.New("test", stubChecker{name: "postgres", delay: time.Minute})
	handler.CheckTimeout = 50 * time.Millisecond

	start := time.Now()
	status, _ := call(t, handler.Ready)
	elapsed := time.Since(start)

	if status != http.StatusServiceUnavailable {
		t.Errorf("want status 503 for a hanging dependency, got %d", status)
	}
	if elapsed > time.Second {
		t.Errorf("readiness probe should time out quickly, took %s", elapsed)
	}
}

func TestReadyWithNoRegisteredCheckersReportsOK(t *testing.T) {
	status, _ := call(t, health.New("test").Ready)

	if status != http.StatusOK {
		t.Errorf("want status 200 when nothing is registered, got %d", status)
	}
}
