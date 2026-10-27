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
