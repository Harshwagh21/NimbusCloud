package user_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/joho/godotenv"

	"github.com/Harshwagh21/NimbusCloud/internal/platform/database/testdb"
	"github.com/Harshwagh21/NimbusCloud/internal/user"
)

func TestMain(m *testing.M) {
	// Local runs load .env; CI already exports TEST_DATABASE_URL.
	_ = godotenv.Load()
	os.Exit(m.Run())
}

func TestCreateRejectsDuplicateEmailIgnoringCase(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateUsers(t, pool)
	repo := user.NewRepository(pool)
	ctx := context.Background()

	_, err := repo.Create(ctx, user.CreateParams{
		Email:        "Harsh@Example.com",
		PasswordHash: "argon2-hash-one",
		Name:         "Harsh",
		StorageLimit: 10_737_418_240,
	})
	if err != nil {
		t.Fatalf("first create should succeed: %v", err)
	}

	_, err = repo.Create(ctx, user.CreateParams{
		Email:        "harsh@example.com",
		PasswordHash: "argon2-hash-two",
		Name:         "Other",
		StorageLimit: 10_737_418_240,
	})
	if !isUniqueViolation(err) {
		t.Fatalf("expected unique-email violation, got: %v", err)
	}
}

func TestCreateThenGetByEmailRoundTrip(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateUsers(t, pool)
	repo := user.NewRepository(pool)
	ctx := context.Background()

	created, err := repo.Create(ctx, user.CreateParams{
		Email:        "reader@nimbus.test",
		PasswordHash: "argon2-hash",
		Name:         "Reader",
		StorageLimit: 10_737_418_240,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	found, err := repo.GetByEmail(ctx, "Reader@Nimbus.Test")
	if err != nil {
		t.Fatalf("get by email: %v", err)
	}
	if found.ID != created.ID {
		t.Fatalf("want id %s, got %s", created.ID, found.ID)
	}
	if found.StorageUsed != 0 {
		t.Fatalf("new user storage_used want 0, got %d", found.StorageUsed)
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
