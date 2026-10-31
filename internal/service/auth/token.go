package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const refreshTokenBytes = 32

type accessTokens struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func (a accessTokens) issue(userID uuid.UUID) (string, error) {
	now := a.now()
	claims := jwt.RegisteredClaims{
		Subject:   userID.String(),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(a.ttl)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(a.secret)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return signed, nil
}

func (a accessTokens) parse(raw string) (uuid.UUID, error) {
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, a.key, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !parsed.Valid {
		return uuid.Nil, err
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, err
	}
	return userID, nil
}

func (a accessTokens) key(token *jwt.Token) (any, error) {
	if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
		return nil, errors.New("unexpected signing method")
	}
	return a.secret, nil
}

func newRefreshToken() (string, string, error) {
	buf := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("refresh token: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	return raw, hashRefreshToken(raw), nil
}

func hashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
