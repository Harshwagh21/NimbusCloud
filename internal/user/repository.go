// Package user owns persistence for accounts. Auth and HTTP live elsewhere;
// this package only talks to the database through sqlc-generated queries.
package user

import (
	"context"

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

// CreateParams is the data needed to insert a user row.
type CreateParams struct {
	Email        string
	PasswordHash string
	Name         string
	StorageLimit int64
}

func (r *Repository) Create(ctx context.Context, params CreateParams) (dbgen.User, error) {
	return r.queries.CreateUser(ctx, dbgen.CreateUserParams{
		Email:        params.Email,
		PasswordHash: params.PasswordHash,
		Name:         params.Name,
		StorageLimit: params.StorageLimit,
	})
}

func (r *Repository) GetByEmail(ctx context.Context, email string) (dbgen.User, error) {
	return r.queries.GetUserByEmail(ctx, email)
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (dbgen.User, error) {
	return r.queries.GetUserByID(ctx, id)
}
