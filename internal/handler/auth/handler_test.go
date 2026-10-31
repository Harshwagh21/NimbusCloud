package auth_test

import (
	"bytes"
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
	"github.com/Harshwagh21/NimbusCloud/middleware"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

func TestRegisterSetsTheRefreshCookieAndOmitsItFromTheBody(t *testing.T) {
	userID := uuid.New()
	script := &scripted{result: authsvc.Result{
		AccessToken:  "access-token",
		ExpiresIn:    900,
		RefreshToken: "refresh-token-secret",
		User:         authsvc.Profile{ID: userID, Email: "ada@nimbus.test", Name: "Ada", StorageLimit: 10},
	}}
	recorder := post(t, router(script, nil), "/api/v1/auth/register", `{"email":"ada@nimbus.test","name":"Ada","password":"correct-horse"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d %s", recorder.Code, recorder.Body.String())
	}
	cookie := refreshCookie(t, recorder)
	if !strings.Contains(cookie, "refresh-token-secret") || strings.Contains(recorder.Body.String(), "refresh-token-secret") {
		t.Fatalf("refresh token leaked or missing: cookie=%s body=%s", cookie, recorder.Body.String())
	}
	for _, flag := range []string{"HttpOnly", "Secure", "SameSite=Lax", "Path=/api/v1/auth"} {
		if !strings.Contains(cookie, flag) {
			t.Errorf("cookie missing %s: %s", flag, cookie)
		}
	}
	if strings.Contains(recorder.Body.String(), "password") {
		t.Fatalf("body included a password field: %s", recorder.Body.String())
	}
}

func TestAuthRoutesMapErrorsAndClearABadRefreshCookie(t *testing.T) {
	script := &scripted{err: apierr.Conflict("EMAIL_TAKEN", "an account with that email already exists")}
	recorder := post(t, router(script, nil), "/api/v1/auth/register", `{"email":"ada@nimbus.test","name":"Ada","password":"correct-horse"}`)
	if recorder.Code != http.StatusConflict || errorCode(t, recorder) != "EMAIL_TAKEN" {
		t.Fatalf("duplicate: %d %s", recorder.Code, recorder.Body.String())
	}

	recorder = post(t, router(script, nil), "/api/v1/auth/register", "{")
	if recorder.Code != http.StatusBadRequest || errorCode(t, recorder) != "VALIDATION_ERROR" {
		t.Fatalf("malformed: %d %s", recorder.Code, recorder.Body.String())
	}

	script.err = apierr.Unauthorized("REFRESH_REUSED", "session is no longer valid")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "nimbus_refresh", Value: "stolen"})
	recorder = httptest.NewRecorder()
	router(script, nil).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnauthorized || errorCode(t, recorder) != "REFRESH_REUSED" {
		t.Fatalf("refresh: %d %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(refreshCookie(t, recorder), "Max-Age=0") {
		t.Fatalf("failed refresh should clear the cookie: %s", recorder.Header().Get("Set-Cookie"))
	}

	script.err = nil
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "nimbus_refresh", Value: "current"})
	recorder = httptest.NewRecorder()
	router(script, nil).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent || script.logout != "current" {
		t.Fatalf("logout: %d token=%s", recorder.Code, script.logout)
	}
}

func TestLoginIsRateLimitedPerClientHost(t *testing.T) {
	script := &scripted{err: apierr.Unauthorized("INVALID_CREDENTIALS", "email or password is incorrect")}
	limit := middleware.NewLimiter(2, time.Minute).Handler()
	engine := router(script, limit)
	for i := range 2 {
		recorder := postFrom(t, engine, "/api/v1/auth/login", `{"email":"ada@nimbus.test","password":"nope"}`, "192.0.2.9:100"+string(rune('0'+i)))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", i, recorder.Code)
		}
	}
	recorder := postFrom(t, engine, "/api/v1/auth/login", `{"email":"ada@nimbus.test","password":"nope"}`, "192.0.2.9:9999")
	if recorder.Code != http.StatusTooManyRequests || errorCode(t, recorder) != "RATE_LIMITED" {
		t.Fatalf("limit: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestMeRequiresABearerToken(t *testing.T) {
	userID := uuid.New()
	script := &scripted{result: authsvc.Result{User: authsvc.Profile{ID: userID, Email: "ada@nimbus.test", Name: "Ada"}}}
	engine := router(script, nil)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	if recorder.Code != http.StatusUnauthorized || errorCode(t, recorder) != "UNAUTHENTICATED" {
		t.Fatalf("missing: %d %s", recorder.Code, recorder.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer good-token")
	recorder = httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "ada@nimbus.test") {
		t.Fatalf("me: %d %s", recorder.Code, recorder.Body.String())
	}
}

func router(script *scripted, limit gin.HandlerFunc) *gin.Engine {
	return server.NewRouter(server.Dependencies{
		Config: config.Config{AppEnv: config.EnvTest, CORSAllowedOrigins: []string{"http://localhost:3000"}},
		Health: health.New("test"),
		Auth:   authhandler.New(script, time.Hour),
		ParseAccessToken: func(token string) (uuid.UUID, error) {
			if token != "good-token" {
				return uuid.Nil, errors.New("bad token")
			}
			return script.result.User.ID, nil
		},
		LimitAuth: limit,
	})
}

func post(t *testing.T, engine *gin.Engine, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return postFrom(t, engine, path, body, "192.0.2.1:1234")
}

func postFrom(t *testing.T, engine *gin.Engine, path, body, remote string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remote
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return recorder
}

func refreshCookie(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	cookie := recorder.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, "nimbus_refresh=") {
		t.Fatalf("missing refresh cookie: %s", cookie)
	}
	return cookie
}

func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("envelope: %v %s", err, recorder.Body.String())
	}
	return body.Error.Code
}

type scripted struct {
	result  authsvc.Result
	err     error
	refresh string
	logout  string
}

func (s *scripted) Register(context.Context, string, string, string) (authsvc.Result, error) {
	return s.result, s.err
}

func (s *scripted) Login(context.Context, string, string) (authsvc.Result, error) {
	return s.result, s.err
}

func (s *scripted) Refresh(_ context.Context, raw string) (authsvc.Result, error) {
	s.refresh = raw
	return s.result, s.err
}

func (s *scripted) Logout(_ context.Context, raw string) error {
	s.logout = raw
	return s.err
}

func (s *scripted) CurrentUser(context.Context, uuid.UUID) (authsvc.Profile, error) {
	return s.result.User, s.err
}
