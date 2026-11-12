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

// UpdateParams replaces name and parent for a live owned folder.
type UpdateParams struct {
	ID       uuid.UUID
	OwnerID  uuid.UUID
	Name     string
	ParentID *uuid.UUID
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

func (r *Repository) Update(ctx context.Context, params UpdateParams) (dbgen.Folder, error) {
	return r.queries.UpdateFolder(ctx, dbgen.UpdateFolderParams{
		ID:       params.ID,
		OwnerID:  params.OwnerID,
		Name:     params.Name,
		ParentID: params.ParentID,
	})
}

func (r *Repository) CountLiveChildren(ctx context.Context, ownerID, folderID uuid.UUID) (int64, error) {
	parentID := folderID
	return r.queries.CountLiveChildren(ctx, dbgen.CountLiveChildrenParams{
		OwnerID:  ownerID,
		ParentID: &parentID,
	})
}

// ListPath returns ancestors from root to the folder (inclusive). Empty if missing.
func (r *Repository) ListPath(ctx context.Context, id, ownerID uuid.UUID) ([]dbgen.Folder, error) {
	rows, err := r.queries.ListFolderPath(ctx, dbgen.ListFolderPathParams{
		ID:      id,
		OwnerID: ownerID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]dbgen.Folder, 0, len(rows))
	for _, row := range rows {
		out = append(out, dbgen.Folder(row))
	}
	return out, nil
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
