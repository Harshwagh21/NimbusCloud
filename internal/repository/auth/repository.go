// Package auth will own session lifecycle. Phase 1 persists refresh-token rows.
// Rotation and revocation run here; hashing, cookies, and HTTP stay in the service
// and handler.
package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "github.com/Harshwagh21/NimbusCloud/db/generated"
)

// Querier is the refresh-token method set shared by a transaction and a fake.
// WithinTx's callback uses this type so the service does not depend on *Repository.
type Querier interface {
	CreateRefreshToken(ctx context.Context, params CreateRefreshTokenParams) (dbgen.RefreshToken, error)
	GetRefreshTokenByHash(ctx context.Context, tokenHash string) (dbgen.RefreshToken, error)
	GetRefreshTokenByHashForUpdate(ctx context.Context, tokenHash string) (dbgen.RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id uuid.UUID) error
	RevokeRefreshTokenFamily(ctx context.Context, familyID uuid.UUID) error
}

// Repository is a thin wrapper over sqlc. No business rules live here.
type Repository struct {
	pool    *pgxpool.Pool
	queries *dbgen.Queries
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: dbgen.New(pool)}
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

// GetRefreshTokenByHashForUpdate locks the row. Call it only inside WithinTx;
// outside a transaction the lock ends with the statement.
func (r *Repository) GetRefreshTokenByHashForUpdate(ctx context.Context, tokenHash string) (dbgen.RefreshToken, error) {
	return r.queries.GetRefreshTokenByHashForUpdate(ctx, tokenHash)
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, id uuid.UUID) error {
	return r.queries.RevokeRefreshToken(ctx, id)
}

func (r *Repository) RevokeRefreshTokenFamily(ctx context.Context, familyID uuid.UUID) error {
	return r.queries.RevokeRefreshTokenFamily(ctx, familyID)
}

// WithinTx runs fn against this repository bound to one transaction.
func (r *Repository) WithinTx(ctx context.Context, fn func(Querier) error) error {
	if r.pool == nil {
		return fn(r)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)

	txRepo := &Repository{queries: r.queries.WithTx(tx)}
	if err := fn(txRepo); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func rollback(tx pgx.Tx) {
	_ = tx.Rollback(context.Background())
}
