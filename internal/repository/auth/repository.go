// Package auth will own session lifecycle. Phase 1 only persists refresh-token
// rows; hashing, cookies, and HTTP arrive in Phase 2.
package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "github.com/Harshwagh21/NimbusCloud/db/generated"
)

// Repository is a thin wrapper over sqlc. No business rules live here.
type Repository struct {
	queries *dbgen.Queries
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{queries: dbgen.New(pool)}
}

// CreateRefreshTokenParams is the data needed to insert a refresh-token row.
// Callers must pass the SHA-256 hash, never the raw token.
type CreateRefreshTokenParams struct {
	UserID    uuid.UUID
	TokenHash string
	FamilyID  uuid.UUID
	ExpiresAt time.Time
}

func (r *Repository) CreateRefreshToken(ctx context.Context, params CreateRefreshTokenParams) (dbgen.RefreshToken, error) {
	return r.queries.CreateRefreshToken(ctx, dbgen.CreateRefreshTokenParams{
		UserID:    params.UserID,
		TokenHash: params.TokenHash,
		FamilyID:  params.FamilyID,
		ExpiresAt: params.ExpiresAt,
	})
}

func (r *Repository) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (dbgen.RefreshToken, error) {
	return r.queries.GetRefreshTokenByHash(ctx, tokenHash)
}

func (r *Repository) RevokeRefreshTokenFamily(ctx context.Context, familyID uuid.UUID) error {
	return r.queries.RevokeRefreshTokenFamily(ctx, familyID)
}
