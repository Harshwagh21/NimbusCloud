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

func TestUpdateRenameAndDuplicate(t *testing.T) {
	store := newFakeStore()
	svc := foldersvc.New(store)
	ownerID := uuid.New()
	ctx := context.Background()

	alpha, err := svc.Create(ctx, ownerID, "Alpha", nil)
	if err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	beta, err := svc.Create(ctx, ownerID, "Beta", nil)
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}

	blank := "   "
	if _, err := svc.Update(ctx, ownerID, alpha.ID, foldersvc.UpdateInput{Name: &blank}); codeOf(err) != "VALIDATION_ERROR" {
		t.Fatalf("blank rename: %v", err)
	}

	newName := "Renamed"
	updated, err := svc.Update(ctx, ownerID, alpha.ID, foldersvc.UpdateInput{Name: &newName})
	if err != nil || updated.Name != "Renamed" {
		t.Fatalf("rename: %+v err=%v", updated, err)
	}

	taken := "Renamed"
	if _, err := svc.Update(ctx, ownerID, beta.ID, foldersvc.UpdateInput{Name: &taken}); codeOf(err) != "FOLDER_NAME_TAKEN" {
		t.Fatalf("duplicate rename: %v", err)
	}
}

func TestUpdateMoveRules(t *testing.T) {
	store := newFakeStore()
	svc := foldersvc.New(store)
	ownerID := uuid.New()
	ctx := context.Background()

	root, _ := svc.Create(ctx, ownerID, "Root", nil)
	rootID := root.ID
	child, _ := svc.Create(ctx, ownerID, "Child", &rootID)
	childID := child.ID
	grand, _ := svc.Create(ctx, ownerID, "Grand", &childID)
	other, _ := svc.Create(ctx, ownerID, "Other", nil)

	missing := uuid.New()
	if _, err := svc.Update(ctx, ownerID, other.ID, foldersvc.UpdateInput{
		ParentID:  &missing,
		ParentSet: true,
	}); codeOf(err) != "FOLDER_NOT_FOUND" {
		t.Fatalf("missing parent: %v", err)
	}

	if _, err := svc.Update(ctx, ownerID, root.ID, foldersvc.UpdateInput{
		ParentID:  &rootID,
		ParentSet: true,
	}); codeOf(err) != "INVALID_FOLDER_MOVE" {
		t.Fatalf("move into self: %v", err)
	}

	if _, err := svc.Update(ctx, ownerID, root.ID, foldersvc.UpdateInput{
		ParentID:  &grand.ID,
		ParentSet: true,
	}); codeOf(err) != "INVALID_FOLDER_MOVE" {
		t.Fatalf("move into descendant: %v", err)
	}

	moved, err := svc.Update(ctx, ownerID, child.ID, foldersvc.UpdateInput{
		ParentID:  &other.ID,
		ParentSet: true,
	})
	if err != nil || moved.ParentID == nil || *moved.ParentID != other.ID {
		t.Fatalf("valid move: %+v err=%v", moved, err)
	}

	toRoot, err := svc.Update(ctx, ownerID, child.ID, foldersvc.UpdateInput{ParentSet: true})
	if err != nil || toRoot.ParentID != nil {
		t.Fatalf("move to root: %+v err=%v", toRoot, err)
	}
}

func TestUpdateMoveRejectsDepthOverflow(t *testing.T) {
	store := newFakeStore()
	svc := foldersvc.New(store)
	ownerID := uuid.New()
	ctx := context.Background()

	var parentID *uuid.UUID
	for i := 0; i < foldersvc.MaxDepth; i++ {
		created, err := svc.Create(ctx, ownerID, nameAt(i), parentID)
		if err != nil {
			t.Fatalf("seed depth %d: %v", i+1, err)
		}
		id := created.ID
		parentID = &id
	}
	leafID := *parentID
	mover, err := svc.Create(ctx, ownerID, "Mover", nil)
	if err != nil {
		t.Fatalf("create mover: %v", err)
	}

	if _, err := svc.Update(ctx, ownerID, mover.ID, foldersvc.UpdateInput{
		ParentID:  &leafID,
		ParentSet: true,
	}); codeOf(err) != "FOLDER_TOO_DEEP" {
		t.Fatalf("want FOLDER_TOO_DEEP, got %v", err)
	}
}

func TestDeleteEmptyAndNotEmpty(t *testing.T) {
	store := newFakeStore()
	svc := foldersvc.New(store)
	ownerID := uuid.New()
	otherOwner := uuid.New()
	ctx := context.Background()

	parent, _ := svc.Create(ctx, ownerID, "Parent", nil)
	parentID := parent.ID
	_, _ = svc.Create(ctx, ownerID, "Child", &parentID)

	if err := svc.Delete(ctx, ownerID, parent.ID); codeOf(err) != "FOLDER_NOT_EMPTY" {
		t.Fatalf("not empty: %v", err)
	}

	empty, _ := svc.Create(ctx, ownerID, "Empty", nil)
	if err := svc.Delete(ctx, ownerID, empty.ID); err != nil {
		t.Fatalf("delete empty: %v", err)
	}
	if _, err := svc.Get(ctx, ownerID, empty.ID); codeOf(err) != "FOLDER_NOT_FOUND" {
		t.Fatalf("deleted get: %v", err)
	}

	if err := svc.Delete(ctx, otherOwner, parent.ID); codeOf(err) != "FOLDER_NOT_FOUND" {
		t.Fatalf("cross-owner delete: %v", err)
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
	return string(rune('A'+(i%26))) + uuid.NewString()[:8]
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

func (f *fakeStore) Update(_ context.Context, params folderrepo.UpdateParams) (dbgen.Folder, error) {
	folder, ok := f.byID[params.ID]
	if !ok || folder.OwnerID != params.OwnerID || folder.DeletedAt != nil {
		return dbgen.Folder{}, pgx.ErrNoRows
	}
	for _, existing := range f.byID {
		if existing.ID == params.ID || existing.DeletedAt != nil {
			continue
		}
		if existing.OwnerID == params.OwnerID &&
			existing.Name == params.Name &&
			sameParent(existing.ParentID, params.ParentID) {
			return dbgen.Folder{}, &pgconn.PgError{Code: "23505"}
		}
	}
	folder.Name = params.Name
	folder.ParentID = params.ParentID
	folder.UpdatedAt = time.Now().UTC()
	f.byID[folder.ID] = folder
	return folder, nil
}

func (f *fakeStore) CountLiveChildren(_ context.Context, ownerID, folderID uuid.UUID) (int64, error) {
	var count int64
	for _, folder := range f.byID {
		if folder.OwnerID == ownerID && folder.DeletedAt == nil &&
			folder.ParentID != nil && *folder.ParentID == folderID {
			count++
		}
	}
	return count, nil
}

func (f *fakeStore) SoftDelete(_ context.Context, id, ownerID uuid.UUID) (dbgen.Folder, error) {
	folder, ok := f.byID[id]
	if !ok || folder.OwnerID != ownerID || folder.DeletedAt != nil {
		return dbgen.Folder{}, pgx.ErrNoRows
	}
	now := time.Now().UTC()
	folder.DeletedAt = &now
	folder.UpdatedAt = now
	f.byID[id] = folder
	return folder, nil
}

func sameParent(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
