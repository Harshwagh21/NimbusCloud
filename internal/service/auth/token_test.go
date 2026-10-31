package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestAccessTokenRoundTrip(t *testing.T) {
	tokens := testTokens(time.Minute)
	userID := uuid.New()

	raw, err := tokens.issue(userID)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	got, err := tokens.parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != userID {
		t.Fatalf("want %s, got %s", userID, got)
	}
}

func TestAccessTokenRejectsExpiryWrongSecretAndOtherAlgorithms(t *testing.T) {
	tokens := testTokens(-time.Minute)
	userID := uuid.New()
	expired, err := tokens.issue(userID)
	if err != nil {
		t.Fatalf("issue expired: %v", err)
	}
	if _, err := tokens.parse(expired); err == nil {
		t.Fatal("expired token was accepted")
	}

	fresh := testTokens(time.Minute)
	raw, err := fresh.issue(userID)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	other := fresh
	other.secret = []byte("a-different-secret-of-sufficient-length")
	if _, err := other.parse(raw); err == nil {
		t.Fatal("token signed with another secret was accepted")
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS384, jwt.RegisteredClaims{
		Subject:   userID.String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}).SignedString(fresh.secret)
	if err != nil {
		t.Fatalf("sign hs384: %v", err)
	}
	if _, err := fresh.parse(signed); err == nil {
		t.Fatal("non-HMAC-SHA256 token was accepted")
	}
}

func testTokens(ttl time.Duration) accessTokens {
	return accessTokens{
		secret: []byte("test-signing-secret-that-is-long-enough"),
		ttl:    ttl,
		now:    time.Now,
	}
}
