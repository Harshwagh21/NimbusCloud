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
	dbgen "github.com/Harshwagh21/NimbusCloud/db/generated"
	authhandler "github.com/Harshwagh21/NimbusCloud/internal/handler/auth"
	folderhandler "github.com/Harshwagh21/NimbusCloud/internal/handler/content/folder"
	"github.com/Harshwagh21/NimbusCloud/internal/health"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/apierr"
	"github.com/Harshwagh21/NimbusCloud/internal/server"
	authsvc "github.com/Harshwagh21/NimbusCloud/internal/service/auth"
	foldersvc "github.com/Harshwagh21/NimbusCloud/internal/service/content/folder"
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

func TestUserCannotAccessAnotherUsersFolders(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()
	folderB := uuid.New()
	gate := &accounts{users: map[uuid.UUID]authsvc.Profile{
		userA: {ID: userA, Email: "a@nimbus.test", Name: "User A"},
		userB: {ID: userB, Email: "b@nimbus.test", Name: "User B"},
	}}
	folders := &ownedFolders{
		byOwner: map[uuid.UUID]dbgen.Folder{
			userB: {ID: folderB, OwnerID: userB, Name: "Secret"},
		},
	}
	engine := server.NewRouter(server.Dependencies{
		Config:           config.Config{AppEnv: config.EnvTest, CORSAllowedOrigins: []string{"http://localhost:3000"}},
		Health:           health.New("test"),
		Auth:             authhandler.New(gate, time.Hour),
		Folders:          folderhandler.New(folders),
		ParseAccessToken: gate.parse,
	})

	tokenA := "Bearer " + userA.String()
	path := "/api/v1/folders/" + folderB.String()

	cases := []struct {
		method string
		url    string
		body   string
	}{
		{method: http.MethodGet, url: path},
		{method: http.MethodGet, url: path + "/path"},
		{method: http.MethodPatch, url: path, body: `{"name":"Hijack"}`},
		{method: http.MethodDelete, url: path},
	}
	for _, tc := range cases {
		recorder := do(engine, tc.method, tc.url, tc.body, tokenA)
		if recorder.Code != http.StatusNotFound || errorCode(t, recorder) != "FOLDER_NOT_FOUND" {
			t.Fatalf("%s %s: want 404 FOLDER_NOT_FOUND, got %d %s",
				tc.method, tc.url, recorder.Code, recorder.Body.String())
		}
	}

	unauth := do(engine, http.MethodGet, path, "", "")
	if unauth.Code != http.StatusUnauthorized || errorCode(t, unauth) != "UNAUTHENTICATED" {
		t.Fatalf("unauthenticated: %d %s", unauth.Code, unauth.Body.String())
	}
}

func getMe(engine *gin.Engine, authorization string) *httptest.ResponseRecorder {
	return do(engine, http.MethodGet, "/api/v1/me", "", authorization)
}

func do(engine *gin.Engine, method, path, body, authorization string) *httptest.ResponseRecorder {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
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

// ownedFolders returns FOLDER_NOT_FOUND unless the caller owns the folder.
type ownedFolders struct {
	byOwner map[uuid.UUID]dbgen.Folder
}

func (o *ownedFolders) lookup(ownerID, id uuid.UUID) (dbgen.Folder, error) {
	folder, ok := o.byOwner[ownerID]
	if !ok || folder.ID != id {
		return dbgen.Folder{}, apierr.NotFound("FOLDER_NOT_FOUND", "folder not found")
	}
	return folder, nil
}

func (o *ownedFolders) Create(context.Context, uuid.UUID, string, *uuid.UUID) (dbgen.Folder, error) {
	return dbgen.Folder{}, apierr.BadRequest("VALIDATION_ERROR", "not used")
}

func (o *ownedFolders) Get(_ context.Context, ownerID, id uuid.UUID) (dbgen.Folder, error) {
	return o.lookup(ownerID, id)
}

func (o *ownedFolders) List(context.Context, uuid.UUID, *uuid.UUID) ([]dbgen.Folder, error) {
	return nil, nil
}

func (o *ownedFolders) Path(_ context.Context, ownerID, id uuid.UUID) ([]dbgen.Folder, error) {
	folder, err := o.lookup(ownerID, id)
	if err != nil {
		return nil, err
	}
	return []dbgen.Folder{folder}, nil
}

func (o *ownedFolders) Update(_ context.Context, ownerID, id uuid.UUID, _ foldersvc.UpdateInput) (dbgen.Folder, error) {
	return o.lookup(ownerID, id)
}

func (o *ownedFolders) Delete(_ context.Context, ownerID, id uuid.UUID) error {
	_, err := o.lookup(ownerID, id)
	return err
}
