package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	dbgen "github.com/Harshwagh21/NimbusCloud/db/generated"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/apierr"
	authrepo "github.com/Harshwagh21/NimbusCloud/internal/repository/auth"
	"github.com/Harshwagh21/NimbusCloud/internal/repository/user"
)

const dummyPassword = "nimbus-timing-placeholder"

// Params is the slice of process config this service needs.
type Params struct {
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	StorageLimit    int64
	Now             func() time.Time
}

// Profile is the account view returned to clients. It has no password hash.
type Profile struct {
	ID           uuid.UUID
	Email        string
	Name         string
	AvatarURL    *string
	StorageUsed  int64
	StorageLimit int64
}

// Result is a new session. RefreshToken is for the cookie, never the JSON body.
type Result struct {
	AccessToken  string
	ExpiresIn    int64
	RefreshToken string
	User         Profile
}

type userStore interface {
	Create(ctx context.Context, params user.CreateParams) (dbgen.User, error)
	GetByEmail(ctx context.Context, email string) (dbgen.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (dbgen.User, error)
}

type sessionStore interface {
	authrepo.Querier
	WithinTx(ctx context.Context, fn func(authrepo.Querier) error) error
}

// Service is the session lifecycle. It does not know about HTTP.
type Service struct {
	users        userStore
	sessions     sessionStore
	tokens       accessTokens
	dummyHash    string
	storageLimit int64
	accessTTL    time.Duration
	refreshTTL   time.Duration
	now          func() time.Time
}

func New(users userStore, sessions sessionStore, params Params) (*Service, error) {
	if err := checkParams(params); err != nil {
		return nil, err
	}
	dummy, err := HashPassword(dummyPassword)
	if err != nil {
		return nil, err
	}
	now := params.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		users:        users,
		sessions:     sessions,
		dummyHash:    dummy,
		storageLimit: params.StorageLimit,
		accessTTL:    params.AccessTokenTTL,
		refreshTTL:   params.RefreshTokenTTL,
		now:          now,
		tokens:       accessTokens{secret: []byte(params.JWTSecret), ttl: params.AccessTokenTTL, now: now},
	}, nil
}

func checkParams(params Params) error {
	if len(params.JWTSecret) < 32 || params.AccessTokenTTL <= 0 || params.RefreshTokenTTL <= 0 || params.StorageLimit <= 0 {
		return errors.New("auth service: incomplete token config")
	}
	return nil
}

// ParseAccessToken returns the user id carried by a bearer token.
func (s *Service) ParseAccessToken(token string) (uuid.UUID, error) {
	return s.tokens.parse(token)
}

func (s *Service) Register(ctx context.Context, email, name, password string) (Result, error) {
	email, name = strings.TrimSpace(email), strings.TrimSpace(name)
	if err := validateRegister(email, name, password); err != nil {
		return Result{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return Result{}, apierr.Internal(err)
	}
	account, err := s.insertUser(ctx, email, name, hash)
	if err != nil {
		return Result{}, err
	}
	return s.startSession(ctx, account)
}

func (s *Service) Login(ctx context.Context, email, password string) (Result, error) {
	email = strings.TrimSpace(email)
	if err := validateLogin(email, password); err != nil {
		return Result{}, err
	}
	account, err := s.users.GetByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return s.rejectUnknown(password)
	}
	if err != nil {
		return Result{}, apierr.Internal(err)
	}
	return s.finishLogin(ctx, account, password)
}

func (s *Service) Refresh(ctx context.Context, rawToken string) (Result, error) {
	if rawToken == "" {
		return Result{}, invalidRefresh()
	}
	var reused bool
	var result Result
	err := s.sessions.WithinTx(ctx, func(q authrepo.Querier) error {
		var rotateErr error
		reused, result, rotateErr = s.rotate(ctx, q, hashRefreshToken(rawToken))
		return rotateErr
	})
	if err != nil {
		return Result{}, err
	}
	if reused {
		return Result{}, refreshReused()
	}
	return result, nil
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	row, err := s.sessions.GetRefreshTokenByHash(ctx, hashRefreshToken(rawToken))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return apierr.Internal(err)
	}
	if err := s.sessions.RevokeRefreshToken(ctx, row.ID); err != nil {
		return apierr.Internal(err)
	}
	return nil
}

func (s *Service) CurrentUser(ctx context.Context, userID uuid.UUID) (Profile, error) {
	account, err := s.users.GetByID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, unauthenticated()
	}
	if err != nil {
		return Profile{}, apierr.Internal(err)
	}
	return profileFrom(account), nil
}
