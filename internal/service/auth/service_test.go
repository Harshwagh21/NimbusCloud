package auth_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	dbgen "github.com/Harshwagh21/NimbusCloud/db/generated"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/apierr"
	"github.com/Harshwagh21/NimbusCloud/internal/repository/auth"
	"github.com/Harshwagh21/NimbusCloud/internal/repository/user"
	authsvc "github.com/Harshwagh21/NimbusCloud/internal/service/auth"
)

func TestRegisterAndLogin(t *testing.T) {
	svc, sessions := newService(t, nil)
	registered := register(t, svc)

	if registered.User.Email != "ada@nimbus.test" || registered.User.Name != "Ada" {
		t.Fatalf("profile: %+v", registered.User)
	}
	if registered.AccessToken == "" || registered.RefreshToken == "" || registered.ExpiresIn != 900 {
		t.Fatalf("session: %+v", registered)
	}
	if got := storedHash(sessions, registered.RefreshToken); got == "" || got == registered.RefreshToken {
		t.Fatal("refresh token must be stored as a hash")
	}
	userID, err := svc.ParseAccessToken(registered.AccessToken)
	if err != nil || userID != registered.User.ID {
		t.Fatalf("access token: id=%s err=%v", userID, err)
	}

	profile, err := svc.CurrentUser(context.Background(), registered.User.ID)
	if err != nil || profile.ID != registered.User.ID {
		t.Fatalf("current user: %+v err=%v", profile, err)
	}

	loggedIn, err := svc.Login(context.Background(), "Ada@Nimbus.Test", "correct-horse")
	if err != nil || loggedIn.User.ID != registered.User.ID {
		t.Fatalf("login: %+v err=%v", loggedIn, err)
	}
}

func TestRegisterRejectsDuplicateInvalidAndWeakInput(t *testing.T) {
	svc, _ := newService(t, nil)
	register(t, svc)

	cases := []struct {
		name     string
		email    string
		userName string
		password string
		code     string
	}{
		{name: "duplicate email", email: "ADA@nimbus.test", userName: "Other", password: "correct-horse", code: "EMAIL_TAKEN"},
		{name: "bad email", email: "not-an-email", userName: "Ada", password: "correct-horse", code: "VALIDATION_ERROR"},
		{name: "missing name", email: "new@nimbus.test", userName: "  ", password: "correct-horse", code: "VALIDATION_ERROR"},
		{name: "short password", email: "new@nimbus.test", userName: "Ada", password: "short", code: "WEAK_PASSWORD"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Register(context.Background(), tc.email, tc.userName, tc.password)
			if codeOf(err) != tc.code {
				t.Fatalf("want %s, got %v", tc.code, err)
			}
		})
	}
}

func TestLoginRejectsUnknownEmailAndWrongPasswordIdentically(t *testing.T) {
	svc, _ := newService(t, nil)
	register(t, svc)

	unknownTook, unknownErr := timedLogin(svc, "missing@nimbus.test", "correct-horse")
	wrongTook, wrongErr := timedLogin(svc, "ada@nimbus.test", "wrong-password")
	if codeOf(unknownErr) != "INVALID_CREDENTIALS" || codeOf(wrongErr) != "INVALID_CREDENTIALS" {
		t.Fatalf("unknown %v wrong %v", unknownErr, wrongErr)
	}
	if unknownTook < time.Millisecond || wrongTook < time.Millisecond {
		t.Fatalf("expected a password hash on both paths, unknown %s wrong %s", unknownTook, wrongTook)
	}
	if unknownTook > wrongTook*4 || wrongTook > unknownTook*4 {
		t.Fatalf("timing diverged: unknown %s wrong %s", unknownTook, wrongTook)
	}
}

func TestRefreshRotatesAndReuseRevokesTheFamily(t *testing.T) {
	svc, _ := newService(t, nil)
	first := register(t, svc)
	second, err := svc.Login(context.Background(), "ada@nimbus.test", "correct-horse")
	if err != nil {
		t.Fatalf("second login: %v", err)
	}

	rotated, err := svc.Refresh(context.Background(), first.RefreshToken)
	if err != nil || rotated.RefreshToken == first.RefreshToken {
		t.Fatalf("rotate: token=%q err=%v", rotated.RefreshToken, err)
	}
	if _, err := svc.Refresh(context.Background(), second.RefreshToken); err != nil {
		t.Fatalf("other family should survive: %v", err)
	}
	if err := mustErr(svc.Refresh(context.Background(), first.RefreshToken)); codeOf(err) != "REFRESH_REUSED" {
		t.Fatalf("replay: %v", err)
	}
	if err := mustErr(svc.Refresh(context.Background(), rotated.RefreshToken)); codeOf(err) != "REFRESH_REUSED" {
		t.Fatalf("successor: %v", err)
	}
}

func TestRefreshOfAnExpiredTokenDoesNotRevokeTheFamily(t *testing.T) {
	now := time.Now()
	svc, _ := newService(t, func() time.Time { return now })
	registered := register(t, svc)

	now = now.Add(2 * time.Hour)
	if err := mustErr(svc.Refresh(context.Background(), registered.RefreshToken)); codeOf(err) != "INVALID_REFRESH_TOKEN" {
		t.Fatalf("expiry: %v", err)
	}
	if err := mustErr(svc.Refresh(context.Background(), registered.RefreshToken)); codeOf(err) != "INVALID_REFRESH_TOKEN" {
		t.Fatalf("second expiry: %v", err)
	}
}

func TestLogoutThenRefreshIsReuse(t *testing.T) {
	svc, _ := newService(t, nil)
	registered := register(t, svc)
	if err := svc.Logout(context.Background(), registered.RefreshToken); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if err := svc.Logout(context.Background(), ""); err != nil {
		t.Fatalf("empty logout: %v", err)
	}
	if err := mustErr(svc.Refresh(context.Background(), registered.RefreshToken)); codeOf(err) != "REFRESH_REUSED" {
		t.Fatalf("logged-out token: %v", err)
	}
}

func TestCurrentUserRejectsAnUnknownID(t *testing.T) {
	svc, _ := newService(t, nil)
	_, err := svc.CurrentUser(context.Background(), uuid.New())
	if codeOf(err) != "UNAUTHENTICATED" {
		t.Fatalf("current user: %v", err)
	}
}

func newService(t *testing.T, now func() time.Time) (*authsvc.Service, *fakeSessions) {
	t.Helper()
	sessions := &fakeSessions{}
	svc, err := authsvc.New(&fakeUsers{
		byEmail: map[string]dbgen.User{},
		byID:    map[uuid.UUID]dbgen.User{},
	}, sessions, authsvc.Params{
		JWTSecret:       strings.Repeat("k", 32),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: time.Hour,
		StorageLimit:    10_737_418_240,
		Now:             now,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc, sessions
}

func register(t *testing.T, svc *authsvc.Service) authsvc.Result {
	t.Helper()
	result, err := svc.Register(context.Background(), "  ada@nimbus.test ", " Ada ", "correct-horse")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return result
}

func timedLogin(svc *authsvc.Service, email, password string) (time.Duration, error) {
	start := time.Now()
	_, err := svc.Login(context.Background(), email, password)
	return time.Since(start), err
}

func codeOf(err error) string {
	if err == nil {
		return ""
	}
	return apierr.From(err).Code
}

func mustErr(_ authsvc.Result, err error) error {
	if err == nil {
		return errors.New("unexpected success")
	}
	return err
}

func storedHash(sessions *fakeSessions, raw string) string {
	sum := sha256.Sum256([]byte(raw))
	want := hex.EncodeToString(sum[:])
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	for _, row := range sessions.rows {
		if row.TokenHash == want {
			return row.TokenHash
		}
	}
	return ""
}

type fakeUsers struct {
	byEmail map[string]dbgen.User
	byID    map[uuid.UUID]dbgen.User
}

func (f *fakeUsers) Create(_ context.Context, params user.CreateParams) (dbgen.User, error) {
	key := strings.ToLower(params.Email)
	if _, ok := f.byEmail[key]; ok {
		return dbgen.User{}, &pgconn.PgError{Code: "23505"}
	}
	account := dbgen.User{
		ID: uuid.New(), Email: params.Email, PasswordHash: params.PasswordHash,
		Name: params.Name, StorageLimit: params.StorageLimit,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	f.byEmail[key] = account
	f.byID[account.ID] = account
	return account, nil
}

func (f *fakeUsers) GetByEmail(_ context.Context, email string) (dbgen.User, error) {
	account, ok := f.byEmail[strings.ToLower(email)]
	if !ok {
		return dbgen.User{}, pgx.ErrNoRows
	}
	return account, nil
}

func (f *fakeUsers) GetByID(_ context.Context, id uuid.UUID) (dbgen.User, error) {
	account, ok := f.byID[id]
	if !ok {
		return dbgen.User{}, pgx.ErrNoRows
	}
	return account, nil
}

type fakeSessions struct {
	mu   sync.Mutex
	rows []dbgen.RefreshToken
}

func (f *fakeSessions) WithinTx(_ context.Context, fn func(auth.Querier) error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return fn(sessionView{f})
}

func (f *fakeSessions) CreateRefreshToken(_ context.Context, params auth.CreateRefreshTokenParams) (dbgen.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.insert(params), nil
}

func (f *fakeSessions) GetRefreshTokenByHash(_ context.Context, tokenHash string) (dbgen.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.find(tokenHash)
}

func (f *fakeSessions) GetRefreshTokenByHashForUpdate(_ context.Context, tokenHash string) (dbgen.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.find(tokenHash)
}

func (f *fakeSessions) RevokeRefreshToken(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revoke(id)
	return nil
}

func (f *fakeSessions) RevokeRefreshTokenFamily(_ context.Context, familyID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revokeFamily(familyID)
	return nil
}

type sessionView struct{ f *fakeSessions }

func (v sessionView) CreateRefreshToken(_ context.Context, params auth.CreateRefreshTokenParams) (dbgen.RefreshToken, error) {
	return v.f.insert(params), nil
}

func (v sessionView) GetRefreshTokenByHash(_ context.Context, tokenHash string) (dbgen.RefreshToken, error) {
	return v.f.find(tokenHash)
}

func (v sessionView) GetRefreshTokenByHashForUpdate(_ context.Context, tokenHash string) (dbgen.RefreshToken, error) {
	return v.f.find(tokenHash)
}

func (v sessionView) RevokeRefreshToken(_ context.Context, id uuid.UUID) error {
	v.f.revoke(id)
	return nil
}

func (v sessionView) RevokeRefreshTokenFamily(_ context.Context, familyID uuid.UUID) error {
	v.f.revokeFamily(familyID)
	return nil
}

func (f *fakeSessions) insert(params auth.CreateRefreshTokenParams) dbgen.RefreshToken {
	row := dbgen.RefreshToken{
		ID: uuid.New(), UserID: params.UserID, TokenHash: params.TokenHash,
		FamilyID: params.FamilyID, ExpiresAt: params.ExpiresAt, CreatedAt: time.Now().UTC(),
	}
	f.rows = append(f.rows, row)
	return row
}

func (f *fakeSessions) find(tokenHash string) (dbgen.RefreshToken, error) {
	for _, row := range f.rows {
		if row.TokenHash == tokenHash {
			return row, nil
		}
	}
	return dbgen.RefreshToken{}, pgx.ErrNoRows
}

func (f *fakeSessions) revoke(id uuid.UUID) {
	now := time.Now().UTC()
	for i := range f.rows {
		if f.rows[i].ID == id && f.rows[i].RevokedAt == nil {
			f.rows[i].RevokedAt = &now
		}
	}
}

func (f *fakeSessions) revokeFamily(familyID uuid.UUID) {
	now := time.Now().UTC()
	for i := range f.rows {
		if f.rows[i].FamilyID == familyID && f.rows[i].RevokedAt == nil {
			f.rows[i].RevokedAt = &now
		}
	}
}
