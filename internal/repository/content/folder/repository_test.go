package folder_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/Harshwagh21/NimbusCloud/internal/platform/database/testdb"
	"github.com/Harshwagh21/NimbusCloud/internal/repository/content/folder"
	"github.com/Harshwagh21/NimbusCloud/internal/repository/user"
)

func TestMain(m *testing.M) {
	_ = godotenv.Load()
	os.Exit(m.Run())
}

func TestCreateRejectsDuplicateRootFolderName(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	ownerID := seedUser(t, pool, "folder-root@nimbus.test")
	repo := folder.NewRepository(pool)
	ctx := context.Background()

	_, err := repo.Create(ctx, folder.CreateParams{
		OwnerID: ownerID,
		Name:    "Docs",
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = repo.Create(ctx, folder.CreateParams{
		OwnerID: ownerID,
		Name:    "Docs",
	})
	if !isUniqueViolation(err) {
		t.Fatalf("expected unique root name violation, got: %v", err)
	}
}

func TestCreateRejectsDuplicateSiblingFolderName(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	ownerID := seedUser(t, pool, "folder-sibling@nimbus.test")
	repo := folder.NewRepository(pool)
	ctx := context.Background()

	parent, err := repo.Create(ctx, folder.CreateParams{
		OwnerID: ownerID,
		Name:    "Parent",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	parentID := parent.ID

	_, err = repo.Create(ctx, folder.CreateParams{
		OwnerID:  ownerID,
		ParentID: &parentID,
		Name:     "Child",
	})
	if err != nil {
		t.Fatalf("first child: %v", err)
	}

	_, err = repo.Create(ctx, folder.CreateParams{
		OwnerID:  ownerID,
		ParentID: &parentID,
		Name:     "Child",
	})
	if !isUniqueViolation(err) {
		t.Fatalf("expected unique sibling name violation, got: %v", err)
	}
}

func TestHardDeleteParentCascadesToChildren(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	ownerID := seedUser(t, pool, "folder-cascade@nimbus.test")
	repo := folder.NewRepository(pool)
	ctx := context.Background()

	parent, err := repo.Create(ctx, folder.CreateParams{
		OwnerID: ownerID,
		Name:    "Parent",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	parentID := parent.ID

	child, err := repo.Create(ctx, folder.CreateParams{
		OwnerID:  ownerID,
		ParentID: &parentID,
		Name:     "Child",
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	if err := repo.HardDelete(ctx, parent.ID, ownerID); err != nil {
		t.Fatalf("hard delete parent: %v", err)
	}

	_, err = repo.GetByID(ctx, child.ID, ownerID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected child gone after cascade, got: %v", err)
	}
}

func TestListByParentExcludesSoftDeletedFolders(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	ownerID := seedUser(t, pool, "folder-softdelete@nimbus.test")
	repo := folder.NewRepository(pool)
	ctx := context.Background()

	live, err := repo.Create(ctx, folder.CreateParams{
		OwnerID: ownerID,
		Name:    "Live",
	})
	if err != nil {
		t.Fatalf("create live: %v", err)
	}
	trashed, err := repo.Create(ctx, folder.CreateParams{
		OwnerID: ownerID,
		Name:    "Trashed",
	})
	if err != nil {
		t.Fatalf("create trash candidate: %v", err)
	}

	if _, err := repo.SoftDelete(ctx, trashed.ID, ownerID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	listed, err := repo.ListByParent(ctx, ownerID, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != live.ID {
		t.Fatalf("want only live folder, got %+v", listed)
	}
}

func TestUpdateRenamesFolder(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	ownerID := seedUser(t, pool, "folder-rename@nimbus.test")
	repo := folder.NewRepository(pool)
	ctx := context.Background()

	created, err := repo.Create(ctx, folder.CreateParams{OwnerID: ownerID, Name: "Old"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := repo.Update(ctx, folder.UpdateParams{
		ID:       created.ID,
		OwnerID:  ownerID,
		Name:     "New",
		ParentID: nil,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "New" {
		t.Fatalf("name = %q, want New", updated.Name)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) && !updated.UpdatedAt.Equal(created.UpdatedAt) {
		t.Fatalf("updated_at should not go backwards: created=%v updated=%v", created.UpdatedAt, updated.UpdatedAt)
	}
}

func TestUpdateRejectsDuplicateSiblingName(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	ownerID := seedUser(t, pool, "folder-update-dup@nimbus.test")
	repo := folder.NewRepository(pool)
	ctx := context.Background()

	if _, err := repo.Create(ctx, folder.CreateParams{OwnerID: ownerID, Name: "Alpha"}); err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	beta, err := repo.Create(ctx, folder.CreateParams{OwnerID: ownerID, Name: "Beta"})
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}

	_, err = repo.Update(ctx, folder.UpdateParams{
		ID:      beta.ID,
		OwnerID: ownerID,
		Name:    "Alpha",
	})
	if !isUniqueViolation(err) {
		t.Fatalf("expected unique violation, got: %v", err)
	}
}

func TestListPathReturnsRootToLeaf(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	ownerID := seedUser(t, pool, "folder-path@nimbus.test")
	repo := folder.NewRepository(pool)
	ctx := context.Background()

	root, err := repo.Create(ctx, folder.CreateParams{OwnerID: ownerID, Name: "Root"})
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	rootID := root.ID
	mid, err := repo.Create(ctx, folder.CreateParams{OwnerID: ownerID, ParentID: &rootID, Name: "Mid"})
	if err != nil {
		t.Fatalf("create mid: %v", err)
	}
	midID := mid.ID
	leaf, err := repo.Create(ctx, folder.CreateParams{OwnerID: ownerID, ParentID: &midID, Name: "Leaf"})
	if err != nil {
		t.Fatalf("create leaf: %v", err)
	}

	path, err := repo.ListPath(ctx, leaf.ID, ownerID)
	if err != nil {
		t.Fatalf("list path: %v", err)
	}
	if len(path) != 3 {
		t.Fatalf("path len = %d, want 3: %+v", len(path), path)
	}
	if path[0].ID != root.ID || path[1].ID != mid.ID || path[2].ID != leaf.ID {
		t.Fatalf("path order wrong: %+v", path)
	}
}

func TestListPathEmptyForOtherOwner(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	ownerA := seedUser(t, pool, "folder-path-a@nimbus.test")
	ownerB := seedUser(t, pool, "folder-path-b@nimbus.test")
	repo := folder.NewRepository(pool)
	ctx := context.Background()

	created, err := repo.Create(ctx, folder.CreateParams{OwnerID: ownerA, Name: "Secret"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	path, err := repo.ListPath(ctx, created.ID, ownerB)
	if err != nil {
		t.Fatalf("list path: %v", err)
	}
	if len(path) != 0 {
		t.Fatalf("expected empty path for other owner, got %+v", path)
	}

	_, err = repo.Update(ctx, folder.UpdateParams{
		ID:      created.ID,
		OwnerID: ownerB,
		Name:    "Hijack",
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected ErrNoRows for cross-owner update, got: %v", err)
	}
}

func TestCountLiveChildrenIgnoresSoftDeleted(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	ownerID := seedUser(t, pool, "folder-count@nimbus.test")
	repo := folder.NewRepository(pool)
	ctx := context.Background()

	parent, err := repo.Create(ctx, folder.CreateParams{OwnerID: ownerID, Name: "Parent"})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	parentID := parent.ID
	live, err := repo.Create(ctx, folder.CreateParams{OwnerID: ownerID, ParentID: &parentID, Name: "Live"})
	if err != nil {
		t.Fatalf("create live: %v", err)
	}
	trashed, err := repo.Create(ctx, folder.CreateParams{OwnerID: ownerID, ParentID: &parentID, Name: "Gone"})
	if err != nil {
		t.Fatalf("create trash: %v", err)
	}
	if _, err := repo.SoftDelete(ctx, trashed.ID, ownerID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	count, err := repo.CountLiveChildren(ctx, ownerID, parent.ID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1 (live=%s)", count, live.ID)
	}
}

func seedUser(t *testing.T, pool *pgxpool.Pool, email string) uuid.UUID {
	t.Helper()
	created, err := user.NewRepository(pool).Create(context.Background(), user.CreateParams{
		Email:        email,
		PasswordHash: "argon2-hash",
		Name:         "Folder Tester",
		StorageLimit: 10_737_418_240,
	})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return created.ID
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
