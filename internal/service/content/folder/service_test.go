package folder_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	dbgen "github.com/Harshwagh21/NimbusCloud/db/generated"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/apierr"
	folderrepo "github.com/Harshwagh21/NimbusCloud/internal/repository/content/folder"
	foldersvc "github.com/Harshwagh21/NimbusCloud/internal/service/content/folder"
)

func TestCreateAtRoot(t *testing.T) {
	store := newFakeStore()
	svc := foldersvc.New(store)
	ownerID := uuid.New()

	created, err := svc.Create(context.Background(), ownerID, "  Docs  ", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Name != "Docs" || created.OwnerID != ownerID || created.ParentID != nil {
		t.Fatalf("unexpected folder: %+v", created)
	}
}

func TestCreateUnderMissingParent(t *testing.T) {
	svc := foldersvc.New(newFakeStore())
	parentID := uuid.New()

	_, err := svc.Create(context.Background(), uuid.New(), "Child", &parentID)
	if codeOf(err) != "FOLDER_NOT_FOUND" {
		t.Fatalf("want FOLDER_NOT_FOUND, got %v", err)
	}
}

func TestCreateRejectsBlankAndDuplicateNames(t *testing.T) {
	store := newFakeStore()
	svc := foldersvc.New(store)
	ownerID := uuid.New()

	if _, err := svc.Create(context.Background(), ownerID, "   ", nil); codeOf(err) != "VALIDATION_ERROR" {
		t.Fatalf("blank: %v", err)
	}

	if _, err := svc.Create(context.Background(), ownerID, "Docs", nil); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := svc.Create(context.Background(), ownerID, "Docs", nil)
	if codeOf(err) != "FOLDER_NAME_TAKEN" {
		t.Fatalf("duplicate: %v", err)
	}
}

func TestCreateRejectsDepthOverflow(t *testing.T) {
	store := newFakeStore()
	svc := foldersvc.New(store)
	ownerID := uuid.New()

	var parentID *uuid.UUID
	for i := 0; i < foldersvc.MaxDepth; i++ {
		created, err := svc.Create(context.Background(), ownerID, nameAt(i), parentID)
		if err != nil {
			t.Fatalf("depth %d: %v", i+1, err)
		}
		id := created.ID
		parentID = &id
	}

	_, err := svc.Create(context.Background(), ownerID, "too-deep", parentID)
	if codeOf(err) != "FOLDER_TOO_DEEP" {
		t.Fatalf("want FOLDER_TOO_DEEP, got %v", err)
	}
}

func TestGetAndList(t *testing.T) {
	store := newFakeStore()
	svc := foldersvc.New(store)
	ownerID := uuid.New()
	otherOwner := uuid.New()

	root, err := svc.Create(context.Background(), ownerID, "Root", nil)
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	rootID := root.ID
	child, err := svc.Create(context.Background(), ownerID, "Child", &rootID)
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	got, err := svc.Get(context.Background(), ownerID, child.ID)
	if err != nil || got.ID != child.ID {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	if _, err := svc.Get(context.Background(), otherOwner, child.ID); codeOf(err) != "FOLDER_NOT_FOUND" {
		t.Fatalf("cross-owner get: %v", err)
	}

	listed, err := svc.List(context.Background(), ownerID, &rootID)
	if err != nil || len(listed) != 1 || listed[0].ID != child.ID {
		t.Fatalf("list children: %+v err=%v", listed, err)
	}
	roots, err := svc.List(context.Background(), ownerID, nil)
	if err != nil || len(roots) != 1 || roots[0].ID != root.ID {
		t.Fatalf("list roots: %+v err=%v", roots, err)
	}
}

func TestPathRootToLeaf(t *testing.T) {
	store := newFakeStore()
	svc := foldersvc.New(store)
	ownerID := uuid.New()

	root, _ := svc.Create(context.Background(), ownerID, "Root", nil)
	rootID := root.ID
	mid, _ := svc.Create(context.Background(), ownerID, "Mid", &rootID)
	midID := mid.ID
	leaf, _ := svc.Create(context.Background(), ownerID, "Leaf", &midID)

	path, err := svc.Path(context.Background(), ownerID, leaf.ID)
	if err != nil || len(path) != 3 {
		t.Fatalf("path: %+v err=%v", path, err)
	}
	if path[0].ID != root.ID || path[2].ID != leaf.ID {
		t.Fatalf("order: %+v", path)
	}
}

func codeOf(err error) string {
	var typed *apierr.Error
	if errors.As(err, &typed) {
		return typed.Code
	}
	return ""
}

func nameAt(i int) string {
	return string(rune('A' + (i % 26))) + uuid.NewString()[:8]
}

type fakeStore struct {
	byID map[uuid.UUID]dbgen.Folder
}

func newFakeStore() *fakeStore {
	return &fakeStore{byID: map[uuid.UUID]dbgen.Folder{}}
}

func (f *fakeStore) Create(_ context.Context, params folderrepo.CreateParams) (dbgen.Folder, error) {
	for _, existing := range f.byID {
		if existing.OwnerID == params.OwnerID &&
			existing.DeletedAt == nil &&
			existing.Name == params.Name &&
			sameParent(existing.ParentID, params.ParentID) {
			return dbgen.Folder{}, &pgconn.PgError{Code: "23505"}
		}
	}
	now := time.Now().UTC()
	folder := dbgen.Folder{
		ID:        uuid.New(),
		OwnerID:   params.OwnerID,
		ParentID:  params.ParentID,
		Name:      params.Name,
		CreatedAt: now,
		UpdatedAt: now,
	}
	f.byID[folder.ID] = folder
	return folder, nil
}

func (f *fakeStore) GetByID(_ context.Context, id, ownerID uuid.UUID) (dbgen.Folder, error) {
	folder, ok := f.byID[id]
	if !ok || folder.OwnerID != ownerID || folder.DeletedAt != nil {
		return dbgen.Folder{}, pgx.ErrNoRows
	}
	return folder, nil
}

func (f *fakeStore) ListByParent(_ context.Context, ownerID uuid.UUID, parentID *uuid.UUID) ([]dbgen.Folder, error) {
	out := make([]dbgen.Folder, 0)
	for _, folder := range f.byID {
		if folder.OwnerID == ownerID && folder.DeletedAt == nil && sameParent(folder.ParentID, parentID) {
			out = append(out, folder)
		}
	}
	return out, nil
}

func (f *fakeStore) ListPath(_ context.Context, id, ownerID uuid.UUID) ([]dbgen.Folder, error) {
	folder, ok := f.byID[id]
	if !ok || folder.OwnerID != ownerID || folder.DeletedAt != nil {
		return nil, nil
	}
	var chain []dbgen.Folder
	for {
		chain = append(chain, folder)
		if folder.ParentID == nil {
			break
		}
		parent, ok := f.byID[*folder.ParentID]
		if !ok || parent.OwnerID != ownerID || parent.DeletedAt != nil {
			return nil, nil
		}
		folder = parent
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

func sameParent(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
