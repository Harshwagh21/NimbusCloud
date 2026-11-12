// Package folder owns folder create/read rules. HTTP lives in the handler package.
package folder

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	dbgen "github.com/Harshwagh21/NimbusCloud/db/generated"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/apierr"
	folderrepo "github.com/Harshwagh21/NimbusCloud/internal/repository/content/folder"
)

// MaxDepth is the deepest allowed folder nesting (root = 1).
const MaxDepth = 20

const maxNameLen = 255

type store interface {
	Create(ctx context.Context, params folderrepo.CreateParams) (dbgen.Folder, error)
	GetByID(ctx context.Context, id, ownerID uuid.UUID) (dbgen.Folder, error)
	ListByParent(ctx context.Context, ownerID uuid.UUID, parentID *uuid.UUID) ([]dbgen.Folder, error)
	ListPath(ctx context.Context, id, ownerID uuid.UUID) ([]dbgen.Folder, error)
}

// Service enforces folder rules without knowing about HTTP.
type Service struct {
	store store
}

func New(store store) *Service {
	return &Service{store: store}
}

func (s *Service) Create(ctx context.Context, ownerID uuid.UUID, name string, parentID *uuid.UUID) (dbgen.Folder, error) {
	name, err := normalizeName(name)
	if err != nil {
		return dbgen.Folder{}, err
	}
	if err := s.ensureParentAllowsChild(ctx, ownerID, parentID); err != nil {
		return dbgen.Folder{}, err
	}
	created, err := s.store.Create(ctx, folderrepo.CreateParams{
		OwnerID:  ownerID,
		ParentID: parentID,
		Name:     name,
	})
	if isUniqueViolation(err) {
		return dbgen.Folder{}, nameTaken()
	}
	if err != nil {
		return dbgen.Folder{}, apierr.Internal(err)
	}
	return created, nil
}

func (s *Service) Get(ctx context.Context, ownerID, id uuid.UUID) (dbgen.Folder, error) {
	folder, err := s.store.GetByID(ctx, id, ownerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.Folder{}, notFound()
	}
	if err != nil {
		return dbgen.Folder{}, apierr.Internal(err)
	}
	return folder, nil
}

func (s *Service) List(ctx context.Context, ownerID uuid.UUID, parentID *uuid.UUID) ([]dbgen.Folder, error) {
	if parentID != nil {
		if _, err := s.Get(ctx, ownerID, *parentID); err != nil {
			return nil, err
		}
	}
	folders, err := s.store.ListByParent(ctx, ownerID, parentID)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return folders, nil
}

func (s *Service) Path(ctx context.Context, ownerID, id uuid.UUID) ([]dbgen.Folder, error) {
	path, err := s.store.ListPath(ctx, id, ownerID)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	if len(path) == 0 {
		return nil, notFound()
	}
	return path, nil
}

func (s *Service) ensureParentAllowsChild(ctx context.Context, ownerID uuid.UUID, parentID *uuid.UUID) error {
	if parentID == nil {
		return nil
	}
	path, err := s.store.ListPath(ctx, *parentID, ownerID)
	if err != nil {
		return apierr.Internal(err)
	}
	if len(path) == 0 {
		return notFound()
	}
	if len(path)+1 > MaxDepth {
		return tooDeep()
	}
	return nil
}

func normalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", apierr.BadRequest("VALIDATION_ERROR", "folder name is required")
	}
	if len(name) > maxNameLen {
		return "", apierr.BadRequest("VALIDATION_ERROR", "folder name is too long")
	}
	return name, nil
}

func notFound() error {
	return apierr.NotFound("FOLDER_NOT_FOUND", "folder not found")
}

func nameTaken() error {
	return apierr.Conflict("FOLDER_NAME_TAKEN", "a folder with that name already exists here")
}

func tooDeep() error {
	return apierr.BadRequest("FOLDER_TOO_DEEP", "folder nesting limit reached")
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
