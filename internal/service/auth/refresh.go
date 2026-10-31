package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	dbgen "github.com/Harshwagh21/NimbusCloud/db/generated"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/apierr"
	authrepo "github.com/Harshwagh21/NimbusCloud/internal/repository/auth"
)

// rotate locks the presented token. A reuse returns reused=true with a nil error so
// WithinTx commits the family revoke; rolling it back would hide the theft.
func (s *Service) rotate(ctx context.Context, q authrepo.Querier, tokenHash string) (bool, Result, error) {
	row, err := q.GetRefreshTokenByHashForUpdate(ctx, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, Result{}, invalidRefresh()
	}
	if err != nil {
		return false, Result{}, apierr.Internal(err)
	}
	if row.RevokedAt != nil {
		return s.markReused(ctx, q, row.FamilyID)
	}
	if !row.ExpiresAt.After(s.now()) {
		return false, Result{}, invalidRefresh()
	}
	result, err := s.replaceToken(ctx, q, row)
	return false, result, err
}

func (s *Service) markReused(ctx context.Context, q authrepo.Querier, familyID uuid.UUID) (bool, Result, error) {
	if err := q.RevokeRefreshTokenFamily(ctx, familyID); err != nil {
		return false, Result{}, apierr.Internal(err)
	}
	return true, Result{}, nil
}

func (s *Service) replaceToken(ctx context.Context, q authrepo.Querier, row dbgen.RefreshToken) (Result, error) {
	if err := q.RevokeRefreshToken(ctx, row.ID); err != nil {
		return Result{}, apierr.Internal(err)
	}
	raw, tokenHash, err := newRefreshToken()
	if err != nil {
		return Result{}, apierr.Internal(err)
	}
	if _, err := s.insertSuccessor(ctx, q, row, tokenHash); err != nil {
		return Result{}, err
	}
	account, err := s.users.GetByID(ctx, row.UserID)
	if err != nil {
		return Result{}, apierr.Internal(err)
	}
	return s.sessionFor(account, raw)
}

func (s *Service) insertSuccessor(ctx context.Context, q authrepo.Querier, row dbgen.RefreshToken, tokenHash string) (dbgen.RefreshToken, error) {
	created, err := q.CreateRefreshToken(ctx, authrepo.CreateRefreshTokenParams{
		UserID:    row.UserID,
		TokenHash: tokenHash,
		FamilyID:  row.FamilyID,
		ExpiresAt: s.now().Add(s.refreshTTL),
	})
	if err != nil {
		return dbgen.RefreshToken{}, apierr.Internal(err)
	}
	return created, nil
}
