package file_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	dbgen "github.com/Harshwagh21/NimbusCloud/db/generated"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/database/testdb"
	"github.com/Harshwagh21/NimbusCloud/internal/repository/content/file"
	"github.com/Harshwagh21/NimbusCloud/internal/repository/user"
)

func TestMain(m *testing.M) {
	_ = godotenv.Load()
	os.Exit(m.Run())
}

func TestListByFolderExcludesSoftDeletedAndNonReadyFiles(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	ownerID := seedUser(t, pool, "file-list@nimbus.test")
	repo := file.NewRepository(pool)
	ctx := context.Background()

	ready, err := repo.Create(ctx, readyFileParams(ownerID, "notes.txt"))
	if err != nil {
		t.Fatalf("create ready: %v", err)
	}
	uploading, err := repo.Create(ctx, file.CreateParams{
		OwnerID:    ownerID,
		Name:       "draft.bin",
		StorageKey: storageKey(ownerID, "draft"),
		MimeType:   "application/octet-stream",
		Size:       10,
		Status:     dbgen.FileStatusUPLOADING,
	})
	if err != nil {
		t.Fatalf("create uploading: %v", err)
	}
	trashed, err := repo.Create(ctx, readyFileParams(ownerID, "old.txt"))
	if err != nil {
		t.Fatalf("create trash candidate: %v", err)
	}
	if _, err := repo.SoftDelete(ctx, trashed.ID, ownerID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	listed, err := repo.ListByFolder(ctx, ownerID, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != ready.ID {
		t.Fatalf("want only ready live file, got %+v (uploading=%s)", listed, uploading.ID)
	}
}

func TestSoftDeletedFileAllowsNameReuse(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	ownerID := seedUser(t, pool, "file-reuse@nimbus.test")
	repo := file.NewRepository(pool)
	ctx := context.Background()

	original, err := repo.Create(ctx, readyFileParams(ownerID, "report.pdf"))
	if err != nil {
		t.Fatalf("create original: %v", err)
	}
	if _, err := repo.SoftDelete(ctx, original.ID, ownerID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	replacement, err := repo.Create(ctx, readyFileParams(ownerID, "report.pdf"))
	if err != nil {
		t.Fatalf("name reuse after trash should succeed: %v", err)
	}
	if replacement.ID == original.ID {
		t.Fatal("replacement should be a new row")
	}
}

func TestCreateRejectsDuplicateActiveFileName(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	ownerID := seedUser(t, pool, "file-dup@nimbus.test")
	repo := file.NewRepository(pool)
	ctx := context.Background()

	_, err := repo.Create(ctx, readyFileParams(ownerID, "same.txt"))
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err = repo.Create(ctx, readyFileParams(ownerID, "same.txt"))
	if !isUniqueViolation(err) {
		t.Fatalf("expected unique name violation, got: %v", err)
	}
}

func TestCreateUploadSessionThenGetByOwner(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	ownerID := seedUser(t, pool, "upload-session@nimbus.test")
	repo := file.NewRepository(pool)
	ctx := context.Background()

	createdFile, err := repo.Create(ctx, file.CreateParams{
		OwnerID:    ownerID,
		Name:       "big.zip",
		StorageKey: storageKey(ownerID, "big"),
		MimeType:   "application/zip",
		Size:       1_048_576,
		Status:     dbgen.FileStatusUPLOADING,
	})
	if err != nil {
		t.Fatalf("create file: %v", err)
	}

	uploadID := "multipart-upload-id"
	session, err := repo.CreateUploadSession(ctx, file.CreateUploadSessionParams{
		UserID:        ownerID,
		FileID:        createdFile.ID,
		UploadID:      &uploadID,
		DeclaredSize:  1_048_576,
		ReservedBytes: 1_048_576,
		PartsUploaded: 0,
		Status:        dbgen.UploadSessionStatusACTIVE,
		ExpiresAt:     time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create upload session: %v", err)
	}

	found, err := repo.GetUploadSessionByID(ctx, session.ID, ownerID)
	if err != nil {
		t.Fatalf("get upload session: %v", err)
	}
	if found.FileID != createdFile.ID {
		t.Fatalf("want file_id %s, got %s", createdFile.ID, found.FileID)
	}
	if found.Status != dbgen.UploadSessionStatusACTIVE {
		t.Fatalf("want ACTIVE, got %s", found.Status)
	}
}

func readyFileParams(ownerID uuid.UUID, name string) file.CreateParams {
	return file.CreateParams{
		OwnerID:    ownerID,
		Name:       name,
		StorageKey: storageKey(ownerID, name),
		MimeType:   "text/plain",
		Size:       32,
		Status:     dbgen.FileStatusREADY,
	}
}

func storageKey(ownerID uuid.UUID, label string) string {
	return fmt.Sprintf("users/%s/files/%s/%s", ownerID, uuid.New(), label)
}

func seedUser(t *testing.T, pool *pgxpool.Pool, email string) uuid.UUID {
	t.Helper()
	created, err := user.NewRepository(pool).Create(context.Background(), user.CreateParams{
		Email:        email,
		PasswordHash: "argon2-hash",
		Name:         "File Tester",
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
