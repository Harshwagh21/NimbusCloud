package file_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
		ID:         uuid.New(),
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
		ID:         uuid.New(),
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

func TestMarkReadyOnlyOnceAndOnlyForOwner(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	ownerID := seedUser(t, pool, "mark-ready@nimbus.test")
	strangerID := seedUser(t, pool, "mark-ready-other@nimbus.test")
	repo := file.NewRepository(pool)
	ctx := context.Background()
	uploading := createUploading(t, repo, ownerID, "photo.png", 10)

	if _, err := repo.MarkReady(ctx, uploading.ID, strangerID, "image/png"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("stranger MarkReady: err = %v, want ErrNoRows", err)
	}
	ready, err := repo.MarkReady(ctx, uploading.ID, ownerID, "image/png")
	if err != nil {
		t.Fatalf("owner MarkReady: %v", err)
	}
	if ready.Status != dbgen.FileStatusREADY || ready.MimeType != "image/png" {
		t.Fatalf("got status %s mime %s", ready.Status, ready.MimeType)
	}
	if _, err := repo.MarkReady(ctx, uploading.ID, ownerID, "image/png"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second MarkReady: err = %v, want ErrNoRows", err)
	}
}

func TestCompleteUploadSessionOnlyOnceAndOnlyForOwner(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	ownerID := seedUser(t, pool, "complete@nimbus.test")
	strangerID := seedUser(t, pool, "complete-other@nimbus.test")
	repo := file.NewRepository(pool)
	ctx := context.Background()
	session := createSession(t, repo, ownerID, createUploading(t, repo, ownerID, "a.txt", 5))

	if _, err := repo.CompleteUploadSession(ctx, session.ID, strangerID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("stranger complete: err = %v, want ErrNoRows", err)
	}
	done, err := repo.CompleteUploadSession(ctx, session.ID, ownerID)
	if err != nil || done.Status != dbgen.UploadSessionStatusCOMPLETED {
		t.Fatalf("owner complete: status %s err %v", done.Status, err)
	}
	if _, err := repo.CompleteUploadSession(ctx, session.ID, ownerID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second complete: err = %v, want ErrNoRows", err)
	}
}

func TestAddStorageUsedStopsAtLimit(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	userID := seedUserWithLimit(t, pool, "quota@nimbus.test", 100)
	repo := file.NewRepository(pool)
	ctx := context.Background()

	steps := []struct {
		name    string
		bytes   int64
		want    int64
		wantErr bool
	}{
		{"fits", 60, 60, false},
		{"would exceed limit, nothing changes", 50, 60, true},
		{"exactly reaches limit", 40, 100, false},
		{"one byte over a full quota", 1, 100, true},
	}
	for _, step := range steps {
		used, err := repo.AddStorageUsed(ctx, userID, step.bytes)
		if step.wantErr {
			if !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("%s: err = %v, want ErrNoRows", step.name, err)
			}
			used = storageUsed(t, pool, userID)
		} else if err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		if used != step.want {
			t.Fatalf("%s: storage_used = %d, want %d", step.name, used, step.want)
		}
	}
}

// Ten completions race for a quota with room for one. A read-then-write check would let
// several through; the single conditional UPDATE lets exactly one through.
func TestAddStorageUsedConcurrentChargesCannotOverbook(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	userID := seedUserWithLimit(t, pool, "race@nimbus.test", 100)
	repo := file.NewRepository(pool)

	const racers = 10
	var wg sync.WaitGroup
	var succeeded atomic.Int32
	for range racers {
		wg.Go(func() {
			if _, err := repo.AddStorageUsed(context.Background(), userID, 60); err == nil {
				succeeded.Add(1)
			}
		})
	}
	wg.Wait()

	if got := succeeded.Load(); got != 1 {
		t.Fatalf("%d of %d charges succeeded, want exactly 1", got, racers)
	}
	if used := storageUsed(t, pool, userID); used != 60 {
		t.Fatalf("storage_used = %d, want 60", used)
	}
}

func TestWithinTxRollsBackEverythingOnError(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	ownerID := seedUser(t, pool, "rollback@nimbus.test")
	repo := file.NewRepository(pool)
	ctx := context.Background()
	params := readyFileParams(ownerID, "ghost.txt")
	boom := errors.New("boom")

	err := repo.WithinTx(ctx, func(q file.Querier) error {
		if _, err := q.Create(ctx, params); err != nil {
			return err
		}
		if _, err := q.AddStorageUsed(ctx, ownerID, 32); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("WithinTx err = %v, want boom", err)
	}
	if _, err := repo.GetByID(ctx, params.ID, ownerID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("file survived rollback: err = %v", err)
	}
	if used := storageUsed(t, pool, ownerID); used != 0 {
		t.Fatalf("storage_used = %d after rollback, want 0", used)
	}
}

func createUploading(t *testing.T, repo *file.Repository, ownerID uuid.UUID, name string, size int64) dbgen.File {
	t.Helper()
	id := uuid.New()
	created, err := repo.Create(context.Background(), file.CreateParams{
		ID:         id,
		OwnerID:    ownerID,
		Name:       name,
		StorageKey: fmt.Sprintf("users/%s/files/%s/%s", ownerID, id, name),
		MimeType:   "application/octet-stream",
		Size:       size,
		Status:     dbgen.FileStatusUPLOADING,
	})
	if err != nil {
		t.Fatalf("create uploading file: %v", err)
	}
	return created
}

func createSession(t *testing.T, repo *file.Repository, ownerID uuid.UUID, f dbgen.File) dbgen.UploadSession {
	t.Helper()
	session, err := repo.CreateUploadSession(context.Background(), file.CreateUploadSessionParams{
		UserID:        ownerID,
		FileID:        f.ID,
		DeclaredSize:  f.Size,
		ReservedBytes: 0,
		Status:        dbgen.UploadSessionStatusACTIVE,
		ExpiresAt:     time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return session
}

func storageUsed(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) int64 {
	t.Helper()
	u, err := user.NewRepository(pool).GetByID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	return u.StorageUsed
}

func readyFileParams(ownerID uuid.UUID, name string) file.CreateParams {
	return file.CreateParams{
		ID:         uuid.New(),
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
	return seedUserWithLimit(t, pool, email, 10_737_418_240)
}

func seedUserWithLimit(t *testing.T, pool *pgxpool.Pool, email string, limit int64) uuid.UUID {
	t.Helper()
	created, err := user.NewRepository(pool).Create(context.Background(), user.CreateParams{
		Email:        email,
		PasswordHash: "argon2-hash",
		Name:         "File Tester",
		StorageLimit: limit,
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
