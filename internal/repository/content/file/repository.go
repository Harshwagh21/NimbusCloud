// Package file owns persistence for file metadata and upload sessions.
// Presigned upload HTTP and quota rules live in later phases.
package file

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

// CreateParams is the data needed to insert a file row.
type CreateParams struct {
	OwnerID    uuid.UUID
	FolderID   *uuid.UUID
	Name       string
	StorageKey string
	MimeType   string
	Size       int64
	Checksum   *string
	Status     dbgen.FileStatus
}

func (r *Repository) Create(ctx context.Context, params CreateParams) (dbgen.File, error) {
	return r.queries.CreateFile(ctx, dbgen.CreateFileParams{
		OwnerID:    params.OwnerID,
		FolderID:   params.FolderID,
		Name:       params.Name,
		StorageKey: params.StorageKey,
		MimeType:   params.MimeType,
		Size:       params.Size,
		Checksum:   params.Checksum,
		Status:     params.Status,
	})
}

func (r *Repository) GetByID(ctx context.Context, id, ownerID uuid.UUID) (dbgen.File, error) {
	return r.queries.GetFileByID(ctx, dbgen.GetFileByIDParams{
		ID:      id,
		OwnerID: ownerID,
	})
}

func (r *Repository) ListByFolder(ctx context.Context, ownerID uuid.UUID, folderID *uuid.UUID) ([]dbgen.File, error) {
	return r.queries.ListFilesByFolder(ctx, dbgen.ListFilesByFolderParams{
		OwnerID:  ownerID,
		FolderID: folderID,
	})
}

func (r *Repository) SoftDelete(ctx context.Context, id, ownerID uuid.UUID) (dbgen.File, error) {
	return r.queries.SoftDeleteFile(ctx, dbgen.SoftDeleteFileParams{
		ID:      id,
		OwnerID: ownerID,
	})
}

// CreateUploadSessionParams is the data needed to track a direct-to-storage upload.
type CreateUploadSessionParams struct {
	UserID        uuid.UUID
	FileID        uuid.UUID
	UploadID      *string
	DeclaredSize  int64
	ReservedBytes int64
	PartsUploaded int32
	Status        dbgen.UploadSessionStatus
	ExpiresAt     time.Time
}

func (r *Repository) CreateUploadSession(ctx context.Context, params CreateUploadSessionParams) (dbgen.UploadSession, error) {
	return r.queries.CreateUploadSession(ctx, dbgen.CreateUploadSessionParams{
		UserID:        params.UserID,
		FileID:        params.FileID,
		UploadID:      params.UploadID,
		DeclaredSize:  params.DeclaredSize,
		ReservedBytes: params.ReservedBytes,
		PartsUploaded: params.PartsUploaded,
		Status:        params.Status,
		ExpiresAt:     params.ExpiresAt,
	})
}

func (r *Repository) GetUploadSessionByID(ctx context.Context, id, userID uuid.UUID) (dbgen.UploadSession, error) {
	return r.queries.GetUploadSessionByID(ctx, dbgen.GetUploadSessionByIDParams{
		ID:     id,
		UserID: userID,
	})
}
