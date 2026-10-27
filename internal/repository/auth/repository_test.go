package auth_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/Harshwagh21/NimbusCloud/internal/platform/database/testdb"
	"github.com/Harshwagh21/NimbusCloud/internal/repository/auth"
	"github.com/Harshwagh21/NimbusCloud/internal/repository/user"
)

func TestMain(m *testing.M) {
	_ = godotenv.Load()
	os.Exit(m.Run())
}

func TestCreateRefreshTokenThenGetByHash(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	userID := seedUser(t, pool, "refresh-lookup@nimbus.test")
	repo := auth.NewRepository(pool)
	ctx := context.Background()

	familyID := uuid.New()
	tokenHash := "sha256-hash-of-opaque-token"
	created, err := repo.CreateRefreshToken(ctx, auth.CreateRefreshTokenParams{
		UserID:    userID,
		TokenHash: tokenHash,
		FamilyID:  familyID,
		ExpiresAt: time.Now().UTC().Add(7 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create refresh token: %v", err)
	}

	found, err := repo.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		t.Fatalf("get by hash: %v", err)
	}
	if found.ID != created.ID {
		t.Fatalf("want id %s, got %s", created.ID, found.ID)
	}
	if found.FamilyID != familyID {
		t.Fatalf("want family %s, got %s", familyID, found.FamilyID)
	}
}

func TestRevokeRefreshTokenFamilyMarksAllActiveRows(t *testing.T) {
	pool := testdb.Open(t)
	testdb.TruncateAll(t, pool)
	userID := seedUser(t, pool, "refresh-revoke@nimbus.test")
	repo := auth.NewRepository(pool)
	ctx := context.Background()
	familyID := uuid.New()
	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour)

	first, err := repo.CreateRefreshToken(ctx, auth.CreateRefreshTokenParams{
		UserID:    userID,
		TokenHash: "hash-one",
		FamilyID:  familyID,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := repo.CreateRefreshToken(ctx, auth.CreateRefreshTokenParams{
		UserID:    userID,
		TokenHash: "hash-two",
		FamilyID:  familyID,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	if err := repo.RevokeRefreshTokenFamily(ctx, familyID); err != nil {
		t.Fatalf("revoke family: %v", err)
	}

	for _, tokenHash := range []string{"hash-one", "hash-two"} {
		found, err := repo.GetRefreshTokenByHash(ctx, tokenHash)
		if err != nil {
			t.Fatalf("get %s: %v", tokenHash, err)
		}
		if found.RevokedAt == nil {
			t.Fatalf("expected %s revoked", tokenHash)
		}
	}
	if first.RevokedAt != nil || second.RevokedAt != nil {
		t.Fatal("create responses should still show pre-revoke state")
	}
}

func seedUser(t *testing.T, pool *pgxpool.Pool, email string) uuid.UUID {
	t.Helper()
	created, err := user.NewRepository(pool).Create(context.Background(), user.CreateParams{
		Email:        email,
		PasswordHash: "argon2-hash",
		Name:         "Auth Tester",
		StorageLimit: 10_737_418_240,
	})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return created.ID
}
