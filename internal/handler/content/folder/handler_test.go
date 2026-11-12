package folder_test

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

func TestCreateReturnsCreatedFolder(t *testing.T) {
	ownerID := uuid.New()
	folderID := uuid.New()
	now := time.Now().UTC()
	script := &scriptedFolders{
		folder: dbgen.Folder{
			ID: folderID, OwnerID: ownerID, Name: "Docs", CreatedAt: now, UpdatedAt: now,
		},
	}
	recorder := do(t, engine(ownerID, script), http.MethodPost, "/api/v1/folders",
		`{"name":"Docs"}`, "Bearer "+ownerID.String())

	if recorder.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"name":"Docs"`) {
		t.Fatalf("body: %s", recorder.Body.String())
	}
	if script.createName != "Docs" || script.createOwner != ownerID {
		t.Fatalf("service args: name=%q owner=%s", script.createName, script.createOwner)
	}
}

func TestListGetPathUpdateDeleteHappyPaths(t *testing.T) {
	ownerID := uuid.New()
	folderID := uuid.New()
	now := time.Now().UTC()
	folder := dbgen.Folder{ID: folderID, OwnerID: ownerID, Name: "Docs", CreatedAt: now, UpdatedAt: now}
	script := &scriptedFolders{folder: folder, folders: []dbgen.Folder{folder}}

	engine := engine(ownerID, script)
	token := "Bearer " + ownerID.String()

	list := do(t, engine, http.MethodGet, "/api/v1/folders", "", token)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"folders"`) {
		t.Fatalf("list: %d %s", list.Code, list.Body.String())
	}

	get := do(t, engine, http.MethodGet, "/api/v1/folders/"+folderID.String(), "", token)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), folderID.String()) {
		t.Fatalf("get: %d %s", get.Code, get.Body.String())
	}

	path := do(t, engine, http.MethodGet, "/api/v1/folders/"+folderID.String()+"/path", "", token)
	if path.Code != http.StatusOK || !strings.Contains(path.Body.String(), `"path"`) {
		t.Fatalf("path: %d %s", path.Code, path.Body.String())
	}

	patch := do(t, engine, http.MethodPatch, "/api/v1/folders/"+folderID.String(),
		`{"name":"Renamed","parent_id":null}`, token)
	if patch.Code != http.StatusOK || !script.updateParentSet || script.updateParent != nil {
		t.Fatalf("patch: %d parentSet=%v parent=%v body=%s",
			patch.Code, script.updateParentSet, script.updateParent, patch.Body.String())
	}

	del := do(t, engine, http.MethodDelete, "/api/v1/folders/"+folderID.String(), "", token)
	if del.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", del.Code, del.Body.String())
	}
}

func TestFolderRoutesRequireAuthAndMapErrors(t *testing.T) {
	ownerID := uuid.New()
	folderID := uuid.New()
	script := &scriptedFolders{err: apierr.NotFound("FOLDER_NOT_FOUND", "folder not found")}
	engine := engine(ownerID, script)

	missing := do(t, engine, http.MethodGet, "/api/v1/folders", "", "")
	if missing.Code != http.StatusUnauthorized || errorCode(t, missing) != "UNAUTHENTICATED" {
		t.Fatalf("missing auth: %d %s", missing.Code, missing.Body.String())
	}

	token := "Bearer " + ownerID.String()
	notFound := do(t, engine, http.MethodGet, "/api/v1/folders/"+folderID.String(), "", token)
	if notFound.Code != http.StatusNotFound || errorCode(t, notFound) != "FOLDER_NOT_FOUND" {
		t.Fatalf("not found: %d %s", notFound.Code, notFound.Body.String())
	}

	badID := do(t, engine, http.MethodGet, "/api/v1/folders/not-a-uuid", "", token)
	if badID.Code != http.StatusBadRequest || errorCode(t, badID) != "VALIDATION_ERROR" {
		t.Fatalf("bad id: %d %s", badID.Code, badID.Body.String())
	}

	badParent := do(t, engine, http.MethodGet, "/api/v1/folders?parent_id=nope", "", token)
	if badParent.Code != http.StatusBadRequest || errorCode(t, badParent) != "VALIDATION_ERROR" {
		t.Fatalf("bad parent query: %d %s", badParent.Code, badParent.Body.String())
	}
}

func engine(ownerID uuid.UUID, folders *scriptedFolders) *gin.Engine {
	auth := &scriptedAuth{user: authsvc.Profile{ID: ownerID, Email: "a@nimbus.test", Name: "A"}}
	return server.NewRouter(server.Dependencies{
		Config:  config.Config{AppEnv: config.EnvTest, CORSAllowedOrigins: []string{"http://localhost:3000"}},
		Health:  health.New("test"),
		Auth:    authhandler.New(auth, time.Hour),
		Folders: folderhandler.New(folders),
		ParseAccessToken: func(token string) (uuid.UUID, error) {
			id, err := uuid.Parse(token)
			if err != nil || id != ownerID {
				return uuid.Nil, errors.New("bad token")
			}
			return id, nil
		},
	})
}

func do(t *testing.T, engine *gin.Engine, method, path, body, auth string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
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
		t.Fatalf("envelope: %v %s", err, recorder.Body.String())
	}
	return body.Error.Code
}

type scriptedFolders struct {
	folder          dbgen.Folder
	folders         []dbgen.Folder
	err             error
	createName      string
	createOwner     uuid.UUID
	updateParentSet bool
	updateParent    *uuid.UUID
}

func (s *scriptedFolders) Create(_ context.Context, ownerID uuid.UUID, name string, _ *uuid.UUID) (dbgen.Folder, error) {
	s.createOwner = ownerID
	s.createName = name
	return s.folder, s.err
}

func (s *scriptedFolders) Get(context.Context, uuid.UUID, uuid.UUID) (dbgen.Folder, error) {
	return s.folder, s.err
}

func (s *scriptedFolders) List(context.Context, uuid.UUID, *uuid.UUID) ([]dbgen.Folder, error) {
	return s.folders, s.err
}

func (s *scriptedFolders) Path(context.Context, uuid.UUID, uuid.UUID) ([]dbgen.Folder, error) {
	return []dbgen.Folder{s.folder}, s.err
}

func (s *scriptedFolders) Update(_ context.Context, _, _ uuid.UUID, input foldersvc.UpdateInput) (dbgen.Folder, error) {
	s.updateParentSet = input.ParentSet
	s.updateParent = input.ParentID
	return s.folder, s.err
}

func (s *scriptedFolders) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return s.err
}

type scriptedAuth struct {
	user authsvc.Profile
}

func (s *scriptedAuth) Register(context.Context, string, string, string) (authsvc.Result, error) {
	return authsvc.Result{}, nil
}
func (s *scriptedAuth) Login(context.Context, string, string) (authsvc.Result, error) {
	return authsvc.Result{}, nil
}
func (s *scriptedAuth) Refresh(context.Context, string) (authsvc.Result, error) {
	return authsvc.Result{}, nil
}
func (s *scriptedAuth) Logout(context.Context, string) error { return nil }
func (s *scriptedAuth) CurrentUser(context.Context, uuid.UUID) (authsvc.Profile, error) {
	return s.user, nil
}
