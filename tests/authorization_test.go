package authorization_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Harshwagh21/NimbusCloud/config"
	authhandler "github.com/Harshwagh21/NimbusCloud/internal/handler/auth"
	"github.com/Harshwagh21/NimbusCloud/internal/health"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/apierr"
	"github.com/Harshwagh21/NimbusCloud/internal/server"
	authsvc "github.com/Harshwagh21/NimbusCloud/internal/service/auth"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

func TestMeReturnsOnlyTheAuthenticatedUser(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()
	gate := &accounts{users: map[uuid.UUID]authsvc.Profile{
		userA: {ID: userA, Email: "a@nimbus.test", Name: "User A"},
		userB: {ID: userB, Email: "b@nimbus.test", Name: "User B"},
	}}
	engine := server.NewRouter(server.Dependencies{
		Config:           config.Config{AppEnv: config.EnvTest, CORSAllowedOrigins: []string{"http://localhost:3000"}},
		Health:           health.New("test"),
		Auth:             authhandler.New(gate, time.Hour),
		ParseAccessToken: gate.parse,
	})

	missing := getMe(engine, "")
	if missing.Code != http.StatusUnauthorized || errorCode(t, missing) != "UNAUTHENTICATED" {
		t.Fatalf("missing token: %d %s", missing.Code, missing.Body.String())
	}

	bad := getMe(engine, "Bearer not-a-token")
	if bad.Code != http.StatusUnauthorized || errorCode(t, bad) != "UNAUTHENTICATED" {
		t.Fatalf("bad token: %d %s", bad.Code, bad.Body.String())
	}

	body := getMe(engine, "Bearer "+userA.String()).Body.String()
	if !strings.Contains(body, "a@nimbus.test") || strings.Contains(body, "b@nimbus.test") {
		t.Fatalf("user A saw the wrong profile: %s", body)
	}
}

func getMe(engine *gin.Engine, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return recorder
}

func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("envelope: %v", err)
	}
	return body.Error.Code
}

type accounts struct {
	users map[uuid.UUID]authsvc.Profile
}

func (a *accounts) parse(token string) (uuid.UUID, error) {
	id, err := uuid.Parse(token)
	if err != nil {
		return uuid.Nil, err
	}
	if _, ok := a.users[id]; !ok {
		return uuid.Nil, errors.New("unknown user")
	}
	return id, nil
}

func (a *accounts) Register(context.Context, string, string, string) (authsvc.Result, error) {
	return authsvc.Result{}, apierr.BadRequest("VALIDATION_ERROR", "not used")
}

func (a *accounts) Login(context.Context, string, string) (authsvc.Result, error) {
	return authsvc.Result{}, apierr.BadRequest("VALIDATION_ERROR", "not used")
}

func (a *accounts) Refresh(context.Context, string) (authsvc.Result, error) {
	return authsvc.Result{}, apierr.Unauthorized("INVALID_REFRESH_TOKEN", "not used")
}

func (a *accounts) Logout(context.Context, string) error { return nil }

func (a *accounts) CurrentUser(_ context.Context, id uuid.UUID) (authsvc.Profile, error) {
	profile, ok := a.users[id]
	if !ok {
		return authsvc.Profile{}, apierr.Unauthorized("UNAUTHENTICATED", "authentication is required")
	}
	return profile, nil
}
