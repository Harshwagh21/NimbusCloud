// Package folder owns persistence for the folder tree. HTTP and move rules live
// in later phases; this package only talks to the database through sqlc.
package folder

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

// CreateParams is the data needed to insert a folder row.
type CreateParams struct {
	OwnerID  uuid.UUID
	ParentID *uuid.UUID
	Name     string
}

func (r *Repository) Create(ctx context.Context, params CreateParams) (dbgen.Folder, error) {
	return r.queries.CreateFolder(ctx, dbgen.CreateFolderParams{
		OwnerID:  params.OwnerID,
		ParentID: params.ParentID,
		Name:     params.Name,
	})
}

func (r *Repository) GetByID(ctx context.Context, id, ownerID uuid.UUID) (dbgen.Folder, error) {
	return r.queries.GetFolderByID(ctx, dbgen.GetFolderByIDParams{
		ID:      id,
		OwnerID: ownerID,
	})
}

func (r *Repository) ListByParent(ctx context.Context, ownerID uuid.UUID, parentID *uuid.UUID) ([]dbgen.Folder, error) {
	return r.queries.ListFoldersByParent(ctx, dbgen.ListFoldersByParentParams{
		OwnerID:  ownerID,
		ParentID: parentID,
	})
}

func (r *Repository) SoftDelete(ctx context.Context, id, ownerID uuid.UUID) (dbgen.Folder, error) {
	return r.queries.SoftDeleteFolder(ctx, dbgen.SoftDeleteFolderParams{
		ID:      id,
		OwnerID: ownerID,
	})
}

func (r *Repository) HardDelete(ctx context.Context, id, ownerID uuid.UUID) error {
	return r.queries.HardDeleteFolder(ctx, dbgen.HardDeleteFolderParams{
		ID:      id,
		OwnerID: ownerID,
	})
}
