package auth

import (
	"context"

	"github.com/google/uuid"

	dbgen "github.com/Harshwagh21/NimbusCloud/db/generated"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/apierr"
	authrepo "github.com/Harshwagh21/NimbusCloud/internal/repository/auth"
	"github.com/Harshwagh21/NimbusCloud/internal/repository/user"
)

func (s *Service) insertUser(ctx context.Context, email, name, hash string) (dbgen.User, error) {
	account, err := s.users.Create(ctx, user.CreateParams{
		Email:        email,
		PasswordHash: hash,
		Name:         name,
		StorageLimit: s.storageLimit,
	})
	if isUniqueViolation(err) {
		return dbgen.User{}, emailTaken()
	}
	if err != nil {
		return dbgen.User{}, apierr.Internal(err)
	}
	return account, nil
}

func (s *Service) rejectUnknown(password string) (Result, error) {
	if _, err := VerifyPassword(password, s.dummyHash); err != nil {
		return Result{}, apierr.Internal(err)
	}
	return Result{}, invalidCredentials()
}

func (s *Service) finishLogin(ctx context.Context, account dbgen.User, password string) (Result, error) {
	match, err := VerifyPassword(password, account.PasswordHash)
	if err != nil {
		return Result{}, apierr.Internal(err)
	}
	if !match {
		return Result{}, invalidCredentials()
	}
	return s.startSession(ctx, account)
}

func (s *Service) startSession(ctx context.Context, account dbgen.User) (Result, error) {
	raw, tokenHash, err := newRefreshToken()
	if err != nil {
		return Result{}, apierr.Internal(err)
	}
	_, err = s.sessions.CreateRefreshToken(ctx, authrepo.CreateRefreshTokenParams{
		UserID:    account.ID,
		TokenHash: tokenHash,
		FamilyID:  uuid.New(),
		ExpiresAt: s.now().Add(s.refreshTTL),
	})
	if err != nil {
		return Result{}, apierr.Internal(err)
	}
	return s.sessionFor(account, raw)
}

func (s *Service) sessionFor(account dbgen.User, rawRefresh string) (Result, error) {
	access, err := s.tokens.issue(account.ID)
	if err != nil {
		return Result{}, apierr.Internal(err)
	}
	return Result{
		AccessToken:  access,
		ExpiresIn:    int64(s.accessTTL.Seconds()),
		RefreshToken: rawRefresh,
		User:         profileFrom(account),
	}, nil
}

func profileFrom(account dbgen.User) Profile {
	return Profile{
		ID:           account.ID,
		Email:        account.Email,
		Name:         account.Name,
		AvatarURL:    account.AvatarUrl,
		StorageUsed:  account.StorageUsed,
		StorageLimit: account.StorageLimit,
	}
}
